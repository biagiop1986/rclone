package fs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDirIno(t *testing.T) {
	d := NewDir("dir", time.Now())
	assert.Equal(t, uint64(0), d.Ino(), "unset")
	assert.Equal(t, uint64(42), d.SetIno(42).Ino())

	// A copy, e.g. the one the VFS makes on rename, keeps it.
	c := NewDirCopy(context.Background(), d).SetRemote("renamed")
	assert.Equal(t, uint64(42), c.Ino())
}
