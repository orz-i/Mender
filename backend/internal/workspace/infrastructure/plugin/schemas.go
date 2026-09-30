package plugin

import "encoding/json"

var workspaceConfigSchema = json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "required":["root"],
  "properties":{
    "root":{
      "type":"string",
      "minLength":1,
      "maxLength":4096,
      "title":"Workspace root",
      "description":"Absolute local directory exposed by this Provider."
    }
  }
}`)

var readInputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["path"],
  "properties":{
    "path":{"type":"string","minLength":1,"maxLength":4096},
    "offset":{"type":"integer","minimum":1,"maximum":1000000000},
    "limit":{"type":"integer","minimum":1,"maximum":2000}
  }
}`)

var readOutputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["path","content","startLine","endLine","truncated"],
  "properties":{
    "path":{"type":"string"},
    "content":{"type":"string"},
    "startLine":{"type":"integer","minimum":0},
    "endLine":{"type":"integer","minimum":0},
    "truncated":{"type":"boolean"}
  }
}`)

var searchInputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["mode"],
  "properties":{
    "mode":{"type":"string","enum":["files","content"]},
    "path":{"type":"string","maxLength":4096},
    "query":{"type":"string","maxLength":4096},
    "pattern":{"type":"string","maxLength":1024},
    "regex":{"type":"boolean"},
    "caseSensitive":{"type":"boolean"},
    "maxResults":{"type":"integer","minimum":1,"maximum":500}
  }
}`)

var searchOutputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["mode","matches","truncated"],
  "properties":{
    "mode":{"type":"string","enum":["files","content"]},
    "matches":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "required":["path"],
        "properties":{
          "path":{"type":"string"},
          "line":{"type":"integer","minimum":1},
          "text":{"type":"string"}
        }
      }
    },
    "truncated":{"type":"boolean"}
  }
}`)

var editInputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["path","operation"],
  "properties":{
    "path":{"type":"string","minLength":1,"maxLength":4096},
    "operation":{"type":"string","enum":["create","replace","delete"]},
    "content":{"type":"string"},
    "edits":{
      "type":"array",
      "maxItems":64,
      "items":{
        "type":"object",
        "additionalProperties":false,
        "required":["oldText","newText"],
        "properties":{
          "oldText":{"type":"string"},
          "newText":{"type":"string"},
          "expectedReplacements":{"type":"integer","minimum":1,"maximum":1000}
        }
      }
    }
  }
}`)

var editOutputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["path","operation","replacements","bytesBefore","bytesAfter"],
  "properties":{
    "path":{"type":"string"},
    "operation":{"type":"string","enum":["create","replace","delete"]},
    "replacements":{"type":"integer","minimum":0},
    "bytesBefore":{"type":"integer","minimum":0},
    "bytesAfter":{"type":"integer","minimum":0}
  }
}`)

var shellInputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["command"],
  "properties":{
    "command":{"type":"string","minLength":1,"maxLength":65536},
    "cwd":{"type":"string","maxLength":4096},
    "timeoutMs":{"type":"integer","minimum":1,"maximum":120000}
  }
}`)

var shellOutputSchema = []byte(`{
  "type":"object",
  "additionalProperties":false,
  "required":["exitCode","stdout","stderr","timedOut","truncated"],
  "properties":{
    "exitCode":{"type":"integer","minimum":-1},
    "stdout":{"type":"string"},
    "stderr":{"type":"string"},
    "timedOut":{"type":"boolean"},
    "truncated":{"type":"boolean"}
  }
}`)
