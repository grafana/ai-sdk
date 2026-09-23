package bedrock

import "encoding/json"

var anthropicProviderToolSchemas = map[string]json.RawMessage{
	"anthropic.advisor_20260301": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {},
  "additionalProperties": false
}`),
	"anthropic.bash_20241022": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "command": {
      "type": "string"
    },
    "restart": {
      "type": "boolean"
    }
  },
  "required": [
    "command"
  ],
  "additionalProperties": false
}`),
	"anthropic.bash_20250124": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "command": {
      "type": "string"
    },
    "restart": {
      "type": "boolean"
    }
  },
  "required": [
    "command"
  ],
  "additionalProperties": false
}`),
	"anthropic.code_execution_20250522": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "code": {
      "type": "string"
    }
  },
  "required": [
    "code"
  ],
  "additionalProperties": false
}`),
	"anthropic.code_execution_20250825": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "anyOf": [
    {
      "type": "object",
      "properties": {
        "type": {
          "type": "string",
          "const": "programmatic-tool-call"
        },
        "code": {
          "type": "string"
        }
      },
      "required": [
        "type",
        "code"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "type": {
          "type": "string",
          "const": "bash_code_execution"
        },
        "command": {
          "type": "string"
        }
      },
      "required": [
        "type",
        "command"
      ],
      "additionalProperties": false
    },
    {
      "anyOf": [
        {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "const": "text_editor_code_execution"
            },
            "command": {
              "type": "string",
              "const": "view"
            },
            "path": {
              "type": "string"
            }
          },
          "required": [
            "type",
            "command",
            "path"
          ],
          "additionalProperties": false
        },
        {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "const": "text_editor_code_execution"
            },
            "command": {
              "type": "string",
              "const": "create"
            },
            "path": {
              "type": "string"
            },
            "file_text": {
              "anyOf": [
                {
                  "type": "string"
                },
                {
                  "type": "null"
                }
              ]
            }
          },
          "required": [
            "type",
            "command",
            "path"
          ],
          "additionalProperties": false
        },
        {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "const": "text_editor_code_execution"
            },
            "command": {
              "type": "string",
              "const": "str_replace"
            },
            "path": {
              "type": "string"
            },
            "old_str": {
              "type": "string"
            },
            "new_str": {
              "type": "string"
            }
          },
          "required": [
            "type",
            "command",
            "path",
            "old_str",
            "new_str"
          ],
          "additionalProperties": false
        }
      ]
    }
  ]
}`),
	"anthropic.code_execution_20260120": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "anyOf": [
    {
      "type": "object",
      "properties": {
        "type": {
          "type": "string",
          "const": "programmatic-tool-call"
        },
        "code": {
          "type": "string"
        }
      },
      "required": [
        "type",
        "code"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "type": {
          "type": "string",
          "const": "bash_code_execution"
        },
        "command": {
          "type": "string"
        }
      },
      "required": [
        "type",
        "command"
      ],
      "additionalProperties": false
    },
    {
      "anyOf": [
        {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "const": "text_editor_code_execution"
            },
            "command": {
              "type": "string",
              "const": "view"
            },
            "path": {
              "type": "string"
            }
          },
          "required": [
            "type",
            "command",
            "path"
          ],
          "additionalProperties": false
        },
        {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "const": "text_editor_code_execution"
            },
            "command": {
              "type": "string",
              "const": "create"
            },
            "path": {
              "type": "string"
            },
            "file_text": {
              "anyOf": [
                {
                  "type": "string"
                },
                {
                  "type": "null"
                }
              ]
            }
          },
          "required": [
            "type",
            "command",
            "path"
          ],
          "additionalProperties": false
        },
        {
          "type": "object",
          "properties": {
            "type": {
              "type": "string",
              "const": "text_editor_code_execution"
            },
            "command": {
              "type": "string",
              "const": "str_replace"
            },
            "path": {
              "type": "string"
            },
            "old_str": {
              "type": "string"
            },
            "new_str": {
              "type": "string"
            }
          },
          "required": [
            "type",
            "command",
            "path",
            "old_str",
            "new_str"
          ],
          "additionalProperties": false
        }
      ]
    }
  ]
}`),
	"anthropic.computer_20241022": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": [
        "key",
        "type",
        "mouse_move",
        "left_click",
        "left_click_drag",
        "right_click",
        "middle_click",
        "double_click",
        "screenshot",
        "cursor_position"
      ]
    },
    "coordinate": {
      "type": "array",
      "items": {
        "type": "integer",
        "minimum": -9007199254740991,
        "maximum": 9007199254740991
      }
    },
    "text": {
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "additionalProperties": false
}`),
	"anthropic.computer_20250124": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": [
        "key",
        "hold_key",
        "type",
        "cursor_position",
        "mouse_move",
        "left_mouse_down",
        "left_mouse_up",
        "left_click",
        "left_click_drag",
        "right_click",
        "middle_click",
        "double_click",
        "triple_click",
        "scroll",
        "wait",
        "screenshot"
      ]
    },
    "coordinate": {
      "type": "array",
      "items": [
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        }
      ]
    },
    "duration": {
      "type": "number"
    },
    "scroll_amount": {
      "type": "number"
    },
    "scroll_direction": {
      "type": "string",
      "enum": [
        "up",
        "down",
        "left",
        "right"
      ]
    },
    "start_coordinate": {
      "type": "array",
      "items": [
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        }
      ]
    },
    "text": {
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "additionalProperties": false
}`),
	"anthropic.computer_20251124": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": [
        "key",
        "hold_key",
        "type",
        "cursor_position",
        "mouse_move",
        "left_mouse_down",
        "left_mouse_up",
        "left_click",
        "left_click_drag",
        "right_click",
        "middle_click",
        "double_click",
        "triple_click",
        "scroll",
        "wait",
        "screenshot",
        "zoom"
      ]
    },
    "coordinate": {
      "type": "array",
      "items": [
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        }
      ]
    },
    "duration": {
      "type": "number"
    },
    "region": {
      "type": "array",
      "items": [
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        }
      ]
    },
    "scroll_amount": {
      "type": "number"
    },
    "scroll_direction": {
      "type": "string",
      "enum": [
        "up",
        "down",
        "left",
        "right"
      ]
    },
    "start_coordinate": {
      "type": "array",
      "items": [
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        },
        {
          "type": "integer",
          "minimum": -9007199254740991,
          "maximum": 9007199254740991
        }
      ]
    },
    "text": {
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "additionalProperties": false
}`),
	"anthropic.memory_20250818": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "anyOf": [
    {
      "type": "object",
      "properties": {
        "command": {
          "type": "string",
          "const": "view"
        },
        "path": {
          "type": "string"
        },
        "view_range": {
          "type": "array",
          "items": [
            {
              "type": "number"
            },
            {
              "type": "number"
            }
          ]
        }
      },
      "required": [
        "command",
        "path"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "command": {
          "type": "string",
          "const": "create"
        },
        "path": {
          "type": "string"
        },
        "file_text": {
          "type": "string"
        }
      },
      "required": [
        "command",
        "path",
        "file_text"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "command": {
          "type": "string",
          "const": "str_replace"
        },
        "path": {
          "type": "string"
        },
        "old_str": {
          "type": "string"
        },
        "new_str": {
          "type": "string"
        }
      },
      "required": [
        "command",
        "path",
        "old_str",
        "new_str"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "command": {
          "type": "string",
          "const": "insert"
        },
        "path": {
          "type": "string"
        },
        "insert_line": {
          "type": "number"
        },
        "insert_text": {
          "type": "string"
        }
      },
      "required": [
        "command",
        "path",
        "insert_line",
        "insert_text"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "command": {
          "type": "string",
          "const": "delete"
        },
        "path": {
          "type": "string"
        }
      },
      "required": [
        "command",
        "path"
      ],
      "additionalProperties": false
    },
    {
      "type": "object",
      "properties": {
        "command": {
          "type": "string",
          "const": "rename"
        },
        "old_path": {
          "type": "string"
        },
        "new_path": {
          "type": "string"
        }
      },
      "required": [
        "command",
        "old_path",
        "new_path"
      ],
      "additionalProperties": false
    }
  ]
}`),
	"anthropic.text_editor_20241022": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "enum": [
        "view",
        "create",
        "str_replace",
        "insert",
        "undo_edit"
      ]
    },
    "path": {
      "type": "string"
    },
    "file_text": {
      "type": "string"
    },
    "insert_line": {
      "type": "integer",
      "minimum": -9007199254740991,
      "maximum": 9007199254740991
    },
    "new_str": {
      "type": "string"
    },
    "insert_text": {
      "type": "string"
    },
    "old_str": {
      "type": "string"
    },
    "view_range": {
      "type": "array",
      "items": {
        "type": "integer",
        "minimum": -9007199254740991,
        "maximum": 9007199254740991
      }
    }
  },
  "required": [
    "command",
    "path"
  ],
  "additionalProperties": false
}`),
	"anthropic.text_editor_20250124": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "enum": [
        "view",
        "create",
        "str_replace",
        "insert",
        "undo_edit"
      ]
    },
    "path": {
      "type": "string"
    },
    "file_text": {
      "type": "string"
    },
    "insert_line": {
      "type": "integer",
      "minimum": -9007199254740991,
      "maximum": 9007199254740991
    },
    "new_str": {
      "type": "string"
    },
    "insert_text": {
      "type": "string"
    },
    "old_str": {
      "type": "string"
    },
    "view_range": {
      "type": "array",
      "items": {
        "type": "integer",
        "minimum": -9007199254740991,
        "maximum": 9007199254740991
      }
    }
  },
  "required": [
    "command",
    "path"
  ],
  "additionalProperties": false
}`),
	"anthropic.text_editor_20250429": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "enum": [
        "view",
        "create",
        "str_replace",
        "insert"
      ]
    },
    "path": {
      "type": "string"
    },
    "file_text": {
      "type": "string"
    },
    "insert_line": {
      "type": "integer",
      "minimum": -9007199254740991,
      "maximum": 9007199254740991
    },
    "new_str": {
      "type": "string"
    },
    "insert_text": {
      "type": "string"
    },
    "old_str": {
      "type": "string"
    },
    "view_range": {
      "type": "array",
      "items": {
        "type": "integer",
        "minimum": -9007199254740991,
        "maximum": 9007199254740991
      }
    }
  },
  "required": [
    "command",
    "path"
  ],
  "additionalProperties": false
}`),
	"anthropic.text_editor_20250728": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "enum": [
        "view",
        "create",
        "str_replace",
        "insert"
      ]
    },
    "path": {
      "type": "string"
    },
    "file_text": {
      "type": "string"
    },
    "insert_line": {
      "type": "integer",
      "minimum": -9007199254740991,
      "maximum": 9007199254740991
    },
    "new_str": {
      "type": "string"
    },
    "insert_text": {
      "type": "string"
    },
    "old_str": {
      "type": "string"
    },
    "view_range": {
      "type": "array",
      "items": {
        "type": "integer",
        "minimum": -9007199254740991,
        "maximum": 9007199254740991
      }
    }
  },
  "required": [
    "command",
    "path"
  ],
  "additionalProperties": false
}`),
	"anthropic.tool_search_bm25_20251119": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "query": {
      "type": "string"
    },
    "limit": {
      "type": "number"
    }
  },
  "required": [
    "query"
  ],
  "additionalProperties": false
}`),
	"anthropic.tool_search_regex_20251119": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string"
    },
    "limit": {
      "type": "number"
    }
  },
  "required": [
    "pattern"
  ],
  "additionalProperties": false
}`),
	"anthropic.web_fetch_20250910": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "url": {
      "type": "string"
    }
  },
  "required": [
    "url"
  ],
  "additionalProperties": false
}`),
	"anthropic.web_fetch_20260209": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "url": {
      "type": "string"
    }
  },
  "required": [
    "url"
  ],
  "additionalProperties": false
}`),
	"anthropic.web_fetch_20260318": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "url": {
      "type": "string"
    }
  },
  "required": [
    "url"
  ],
  "additionalProperties": false
}`),
	"anthropic.web_search_20250305": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "query": {
      "type": "string"
    }
  },
  "required": [
    "query"
  ],
  "additionalProperties": false
}`),
	"anthropic.web_search_20260209": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "query": {
      "type": "string"
    }
  },
  "required": [
    "query"
  ],
  "additionalProperties": false
}`),
	"anthropic.web_search_20260318": json.RawMessage(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "query": {
      "type": "string"
    }
  },
  "required": [
    "query"
  ],
  "additionalProperties": false
}`),
}
