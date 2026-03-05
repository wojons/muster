package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun_HelpAction(t *testing.T) {
	os.Args = []string{"muster-daemon"}

	err := run()
	require.NoError(t, err, "no action should show help")
}

func TestRun_StatusAction(t *testing.T) {
	os.Args = []string{"muster-daemon", "status"}

	err := run()
	require.NoError(t, err, "status command should succeed")
}
