package v1

import "testing"

import "github.com/stretchr/testify/require"

func TestRerankRequestPrefix(t *testing.T) {
	req := RerankRequest{Query: "hello world"}
	require.Equal(t, "hello", req.Prefix(5))
}
