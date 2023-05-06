package main

import "testing"

const (
	expectOk    = true
	expectError = false
)

type gatewayNameTestCase struct {
	name           string
	gatewayName    string
	expectedResult bool
}

var gatewayNameTestTable = []gatewayNameTestCase{
	{"empty", "", expectError},
	{"blank", "   ", expectError},
	{"alpha", "abc", expectOk},
	{"num", "123", expectOk},
	{"alpha-num", "abc123", expectOk},
	{"colon", "abc:123", expectOk},
	{"spaced", "abc 123", expectError},
	{"single $", "$", expectError},
	{"interpolated $", "abc$123", expectError},
	{"$ plus space", "abc $123", expectError},
	{"{ isolated", "{", expectError},
	{"{ interpolated", "a{b", expectError},
	{"} isolated", "}", expectError},
	{"} interpolated", "a}b", expectError},
	{"{}", "{}", expectError},
	{"a{}b", "a{}b", expectError},
	{"{ab}", "{ab}", expectError},
	{"}{", "}{", expectError},
	{"a}{b", "a}{b", expectError},
	{"}ab{", "}ab{", expectError},
	{"all invalid chars", " {}$", expectError},
	{"some valid chars", ",:/a3-_[]", expectOk},
}

func TestGatewayName(t *testing.T) {
	for _, data := range gatewayNameTestTable {
		errVal := validateInputGatewayName(data.gatewayName)
		ok := errVal == nil
		if ok != data.expectedResult {
			t.Errorf("%s: expected=%t got=%t error:%v",
				data.name, data.expectedResult, ok, errVal)
		}
	}
}
