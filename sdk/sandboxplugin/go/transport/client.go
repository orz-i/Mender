package transport

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
	"google.golang.org/grpc"
)

// ClientOptions configures one Mender-owned native Sandbox plugin process.
type ClientOptions struct {
	Executable          string
	WorkingDirectory    string
	Versions            []uint32
	ExecutableChecksum  []byte
	StartTimeout        time.Duration
	Stderr              io.Writer
	PluginLogBufferSize int
	Command             *exec.Cmd
}

// Client owns one native Sandbox plugin process and its gRPC service client.
type Client struct {
	process *plugin.Client
	service sandboxv1.SandboxPluginServiceClient
	version uint32
}

// StartClient launches and connects to one native Sandbox plugin process.
func StartClient(options ClientOptions) (*Client, error) {
	if !filepath.IsAbs(options.Executable) {
		return nil, fmt.Errorf("sandbox plugin transport: executable path must be absolute")
	}
	if options.WorkingDirectory == "" || !filepath.IsAbs(options.WorkingDirectory) {
		return nil, fmt.Errorf("sandbox plugin transport: working directory must be absolute")
	}
	if len(options.ExecutableChecksum) != sha256.Size {
		return nil, fmt.Errorf(
			"sandbox plugin transport: executable checksum must be %d bytes",
			sha256.Size,
		)
	}
	if err := verifyExecutableChecksum(options.Executable, options.ExecutableChecksum); err != nil {
		return nil, err
	}
	sets, err := VersionedPluginSets(options.Versions, nil)
	if err != nil {
		return nil, err
	}

	command := options.Command
	var secureConfig *plugin.SecureConfig
	if command == nil {
		command = exec.Command(options.Executable) // #nosec G204 -- exact managed executable, no shell.
		secureConfig = &plugin.SecureConfig{
			Checksum: append([]byte(nil), options.ExecutableChecksum...),
			Hash:     sha256.New(),
		}
	}
	if command.Path == "" {
		return nil, fmt.Errorf("sandbox plugin transport: command path must not be empty")
	}
	command.Dir = options.WorkingDirectory

	process := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:     Handshake,
		VersionedPlugins:    sets,
		Cmd:                 command,
		SecureConfig:        secureConfig,
		Managed:             false,
		StartTimeout:        options.StartTimeout,
		Stderr:              options.Stderr,
		SyncStdout:          io.Discard,
		SyncStderr:          io.Discard,
		AllowedProtocols:    []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:              hclog.NewNullLogger(),
		PluginLogBufferSize: options.PluginLogBufferSize,
		AutoMTLS:            true,
		SkipHostEnv:         true,
		GRPCBrokerMultiplex: false,
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(MaxMessageBytes),
				grpc.MaxCallSendMsgSize(MaxMessageBytes),
			),
		},
	})
	rpcClient, err := process.Client()
	if err != nil {
		process.Kill()
		return nil, fmt.Errorf("sandbox plugin transport: start client: %w", err)
	}
	raw, err := rpcClient.Dispense(PluginName)
	if err != nil {
		process.Kill()
		return nil, fmt.Errorf("sandbox plugin transport: dispense service: %w", err)
	}
	service, ok := raw.(sandboxv1.SandboxPluginServiceClient)
	if !ok {
		process.Kill()
		return nil, fmt.Errorf("sandbox plugin transport: unexpected service client %T", raw)
	}
	version := process.NegotiatedVersion()
	if version < 1 {
		process.Kill()
		return nil, fmt.Errorf("sandbox plugin transport: invalid negotiated version %d", version)
	}
	return &Client{
		process: process,
		service: service,
		version: uint32(version), // #nosec G115 -- negotiated version comes from VersionedPluginSets, bounded to MaxInt32.
	}, nil
}

func verifyExecutableChecksum(path string, expected []byte) error {
	file, err := os.Open(path) // #nosec G304 -- path is the host-resolved managed plugin executable already required to be absolute.
	if err != nil {
		return fmt.Errorf("sandbox plugin transport: open executable for checksum: %w", err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("sandbox plugin transport: hash executable: %w", err)
	}
	actual := hash.Sum(nil)
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("sandbox plugin transport: executable checksum mismatch")
	}
	return nil
}

// Service returns the connected Sandbox Plugin Protocol client.
func (c *Client) Service() sandboxv1.SandboxPluginServiceClient {
	if c == nil {
		return nil
	}
	return c.service
}

// ProtocolVersion returns the negotiated Mender Sandbox Plugin Protocol version.
func (c *Client) ProtocolVersion() uint32 {
	if c == nil {
		return 0
	}
	return c.version
}

// Exited reports whether the plugin process has terminated.
func (c *Client) Exited() bool {
	return c == nil || c.process == nil || c.process.Exited()
}

// Close terminates the plugin process and releases transport resources.
func (c *Client) Close() error {
	if c == nil || c.process == nil {
		return nil
	}
	c.process.Kill()
	return nil
}
