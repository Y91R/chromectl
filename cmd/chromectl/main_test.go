package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersion_PrintsVersion(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer

	code := run([]string{"version"}, &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "dev\n", stdout.String())
}

func TestVersion_JSON(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer

	code := run([]string{"version", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.JSONEq(t, `{"version":"dev"}`, stdout.String())
}

func TestUnknownCommand_FailsWithMessageInStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer

	code := run([]string{"nope"}, &stdout, &stderr)

	assert.NotEqual(t, 0, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "nope")
}
