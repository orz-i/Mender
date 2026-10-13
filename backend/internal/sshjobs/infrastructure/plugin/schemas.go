package plugin

import "encoding/json"

var facadeConfigSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["sandboxId"],"properties":{"sandboxId":{"type":"string","pattern":"^sbx_[0-9a-f]{32}$"}}}`)

var submitInputSchema = []byte(`{"type":"object","additionalProperties":false,"required":["idempotencyKey","command"],"properties":{"idempotencyKey":{"type":"string","pattern":"^[0-9a-f]{32}$","description":"Client-generated stable random key reused only for retry of this exact command"},"command":{"type":"string","minLength":1,"maxLength":65536}}}`)
var taskInputSchema = []byte(`{"type":"object","additionalProperties":false,"required":["taskId"],"properties":{"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"}}}`)
var logInputSchema = []byte(`{"type":"object","additionalProperties":false,"required":["taskId"],"properties":{"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"},"stream":{"type":"string","enum":["stdout","stderr"]},"cursor":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":65536}}}`)
var taskOutputSchema = []byte(`{"type":"object","additionalProperties":false,"required":["taskId","state","cancelRequested"],"properties":{"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"},"state":{"type":"string","enum":["running","completed","failed","cancelled","unknown"]},"unit":{"type":"string"},"cancelRequested":{"type":"boolean"},"exitCode":{"type":"integer","minimum":0,"maximum":255}}}`)
var logOutputSchema = []byte(`{"type":"object","additionalProperties":false,"required":["taskId","stream","content","cursor","truncated"],"properties":{"taskId":{"type":"string"},"stream":{"type":"string","enum":["stdout","stderr"]},"content":{"type":"string"},"cursor":{"type":"integer","minimum":0},"truncated":{"type":"boolean"}}}`)
