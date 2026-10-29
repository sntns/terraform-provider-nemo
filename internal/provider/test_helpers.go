package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadTestConfiguration reads the configuration of an acceptance test, in
// testdata/resources/<resource>/<test>.tf.
func loadTestConfiguration(t *testing.T, resourceName, testName string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "resources", resourceName, testName+".tf"))
	require.NoError(t, err)
	return string(data)
}
