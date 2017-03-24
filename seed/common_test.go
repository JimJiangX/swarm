package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommon(t *testing.T) {
	assert.True(t, checkMount("/dev"))
	assert.False(t, checkMount("/TEST11"))
}
