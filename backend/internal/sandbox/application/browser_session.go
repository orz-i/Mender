package application

import "context"

// BrowserSession is intentionally separate from WorkspaceSession. It is only
// an integration contract: Mender currently has NO registered implementation
// or execution router for a managed browser session.
//
// Future adapters must authenticate/authorize owner+Profile, enforce site
// permissions and navigation/network constraints, acquire a resource-specific
// lease, isolate cookies and sessions, and audit all operations before opening
// a BrowserSession. A browser URL/selector must never become a host file path.
type BrowserSession interface {
	Session
	Navigate(context.Context, BrowserNavigation) (BrowserSnapshot, error)
	Snapshot(context.Context) (BrowserSnapshot, error)
	Click(context.Context, BrowserTarget) (BrowserSnapshot, error)
	Type(context.Context, BrowserInput) (BrowserSnapshot, error)
	Screenshot(context.Context) (BrowserImage, error)
}

type BrowserNavigation struct{ URL string }
type BrowserTarget struct{ Selector string }
type BrowserInput struct {
	Target BrowserTarget
	Text   string
}
type BrowserSnapshot struct{ Content string }
type BrowserImage struct {
	MimeType string
	Data     []byte
}
