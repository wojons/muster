package openapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultParseOptions(t *testing.T) {
	opts := DefaultParseOptions()
	require.Equal(t, 10, opts.MaxDepth)
	require.True(t, opts.AllowCircular)
	require.False(t, opts.SkipValidation)
}

func TestParser_Parse_ValidSpec(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/test": {
				"get": {
					"summary": "Test endpoint",
					"operationId": "testGet",
					"responses": {
						"200": {
							"description": "Success",
							"content": {
								"application/json": {
									"schema": {"type": "object"}
								}
							}
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Document)
	require.Equal(t, "1.0.0", result.Version)
	require.Greater(t, result.ParseTime, int64(0))
}

func TestParser_Parse_InvalidSpec(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	invalidSpec := []byte(`{"invalid": "spec"}`)

	_, err := parser.Parse(ctx, invalidSpec, DefaultParseOptions())
	require.Error(t, err)
}

func TestParser_Parse_WithValidation(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	invalidSpec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test"
		},
		"paths": {}
	}`)

	_, err := parser.Parse(ctx, invalidSpec, DefaultParseOptions())
	require.Error(t, err)
}

func TestParser_Parse_SkipValidation(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	opts := DefaultParseOptions()
	opts.SkipValidation = true

	invalidSpec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test"
		},
		"paths": {}
	}`)

	result, err := parser.Parse(ctx, invalidSpec, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestParser_MissingTitleWarning(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()
	opts := DefaultParseOptions()
	opts.SkipValidation = true

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "",
			"version": "1.0.0"
		},
		"paths": {}
	}`)

	result, err := parser.Parse(ctx, spec, opts)
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "semantic", result.Warnings[0].Type)
}

func TestParser_ExtractOperations_Simple(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/test": {
				"get": {
					"summary": "Test endpoint",
					"operationId": "testGet",
					"responses": {
						"200": {
							"description": "Success",
							"content": {
								"application/json": {
									"schema": {"type": "object"}
								}
							}
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	require.Equal(t, "testGet", op.ID)
	require.Equal(t, "Test endpoint", op.Summary)
	require.Equal(t, "GET", op.Method)
	require.Equal(t, "/test", op.Path)
}

func TestParser_ExtractOperations_AllMethods(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/test": {
				"get": {
					"operationId": "testGet",
					"responses": {"200": {"description": "Success"}}
				},
				"post": {
					"operationId": "testPost",
					"responses": {"201": {"description": "Created"}}
				},
				"put": {
					"operationId": "testPut",
					"responses": {"200": {"description": "Success"}}
				},
				"delete": {
					"operationId": "testDelete",
					"responses": {"204": {"description": "No Content"}}
				},
				"patch": {
					"operationId": "testPatch",
					"responses": {"200": {"description": "Success"}}
				},
				"head": {
					"operationId": "testHead",
					"responses": {"200": {"description": "Success"}}
				},
				"options": {
					"operationId": "testOptions",
					"responses": {"200": {"description": "Success"}}
				},
				"trace": {
					"operationId": "testTrace",
					"responses": {"200": {"description": "Success"}}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 8)
}

func TestParser_ExtractOperation_WithParameters(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/users/{id}": {
				"get": {
					"operationId": "getUser",
					"parameters": [
						{
							"name": "id",
							"in": "path",
							"required": true,
							"schema": {"type": "string"}
						},
						{
							"name": "fields",
							"in": "query",
							"required": false,
							"schema": {"type": "string"}
						}
					],
					"responses": {
						"200": {
							"description": "Success"
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	require.Len(t, op.Parameters, 2)

	pathParam := op.Parameters[0]
	require.Equal(t, "id", pathParam.Name)
	require.Equal(t, "path", pathParam.In)
	require.True(t, pathParam.Required)

	queryParam := op.Parameters[1]
	require.Equal(t, "fields", queryParam.Name)
	require.Equal(t, "query", queryParam.In)
	require.False(t, queryParam.Required)
}

func TestParser_ExtractOperation_WithRequestBody(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/users": {
				"post": {
					"operationId": "createUser",
					"requestBody": {
						"description": "User data",
						"required": true,
						"content": {
							"application/json": {
								"schema": {"type": "object"}
							}
						}
					},
					"responses": {
						"201": {
							"description": "Created"
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	require.NotNil(t, op.RequestBody)
	require.True(t, op.RequestBody.Required)
	require.Contains(t, op.RequestBody.Content, "application/json")
}

func TestParser_ExtractOperation_WithoutRequestBody(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"responses": {
						"200": {
							"description": "Success"
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	require.Nil(t, ops[0].RequestBody)
}

func TestParser_ExtractOperation_WithTags(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"tags": ["users", "list"],
					"responses": {
						"200": {
							"description": "Success"
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	require.Equal(t, []string{"users", "list"}, op.Tags)
}

func TestParser_ExtractOperation_EmptySpec(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Empty(t, ops)
}

func TestParser_ExtractOperation_NullOperation(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/test": {}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Empty(t, ops)
}

func TestParser_ExtractOperation_Types(t *testing.T) {
	ctx := context.Background()
	parser := NewParser()

	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/test": {
				"get": {
					"operationId": "testOp",
					"parameters": [
						{
							"name": "boolParam",
							"in": "query",
							"schema": {"type": "boolean"}
						},
						{
							"name": "intParam",
							"in": "query",
							"schema": {"type": "integer"}
						},
						{
							"name": "strParam",
							"in": "query",
							"schema": {"type": "string"}
						}
					],
					"responses": {
						"200": {
							"description": "Success"
						}
					}
				}
			}
		}
	}`)

	result, err := parser.Parse(ctx, spec, DefaultParseOptions())
	require.NoError(t, err)

	ops, err := parser.ExtractOperations(result.Document)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	require.Len(t, op.Parameters, 3)
	require.Equal(t, "boolean", op.Parameters[0].Type)
	require.Equal(t, "integer", op.Parameters[1].Type)
	require.Equal(t, "string", op.Parameters[2].Type)
}
