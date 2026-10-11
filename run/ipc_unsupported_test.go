//go:build !linux && !windows && !darwin

package run

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsupportedPlatformIsExplicit(t *testing.T) {
	_, err := userIdentity()
	require.ErrorContains(t, err, "only Linux, Windows and macOS")
}
