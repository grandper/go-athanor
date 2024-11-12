package grpc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
)

func TestRespondWithError(t *testing.T) {
	t.Run("should return a gRPC status error carrying the code and the error message", func(t *testing.T) {
		err := grpcserver.RespondWithError(codes.NotFound, assert.AnError)

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Equal(t, assert.AnError.Error(), st.Message())
	})

	t.Run("should keep a code outside the standard list", func(t *testing.T) {
		err := grpcserver.RespondWithError(codes.Code(42), assert.AnError)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Code(42), st.Code())
		assert.Equal(t, assert.AnError.Error(), st.Message())
	})
}
