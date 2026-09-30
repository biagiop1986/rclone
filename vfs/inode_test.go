package vfs

import (
	"context"
	"testing"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fstest/mockfs"
	"github.com/rclone/rclone/fstest/mockobject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inoObject wraps a mock object with a backend 64-bit id so it satisfies
// fs.Inoer.
type inoObject struct {
	fs.Object
	ino uint64
}

// Ino returns the backend id of the object
func (o inoObject) Ino() uint64 { return o.ino }

func newInoObject(remote string, ino uint64) inoObject {
	return inoObject{
		Object: mockobject.New(remote).WithContent([]byte("x"), mockobject.SeekModeNone),
		ino:    ino,
	}
}

func TestDeriveInode(t *testing.T) {
	t.Run("BackendID", func(t *testing.T) {
		assert.Equal(t, uint64(42), deriveInode(newInoObject("file.txt", 42)))
	})

	// Two objects with the same id - as when the VFS drops a node and
	// rebuilds it from a fresh listing - get the same inode.
	t.Run("SameIDSameInode", func(t *testing.T) {
		a := deriveInode(newInoObject("file.txt", 1234))
		b := deriveInode(newInoObject("file.txt", 1234))
		assert.Equal(t, a, b)
	})

	// 0 means "no id", and 1 is the mount root: both fall back to the
	// counter, which never hands out 0 or 1 either.
	t.Run("ReservedIDsFallBack", func(t *testing.T) {
		for _, ino := range []uint64{0, 1} {
			got := deriveInode(newInoObject("file.txt", ino))
			assert.Greater(t, got, uint64(1), "id %d", ino)
		}
	})

	t.Run("NoIDUsesCounter", func(t *testing.T) {
		a := deriveInode(mockobject.New("file.txt"))
		b := deriveInode(mockobject.New("file.txt"))
		assert.NotEqual(t, a, b)
		assert.Greater(t, a, uint64(1))
		assert.Greater(t, deriveInode(nil), uint64(1))
	})
}

// A file keeps its inode number when the directory cache forgets it and
// lists it again, as long as the backend identifies it.
func TestFileInodeAfterForget(t *testing.T) {
	inodeAfterForget := func(t *testing.T, obj fs.Object) (before, after uint64) {
		fMock, err := mockfs.NewFs(context.Background(), "test", "root", nil)
		require.NoError(t, err)
		fMock.(*mockfs.Fs).AddObject(obj)
		vfs := New(context.Background(), fMock, nil)
		t.Cleanup(func() { cleanupVFS(t, vfs) })

		node, err := vfs.Stat(obj.Remote())
		require.NoError(t, err)
		before = node.Inode()
		root, err := vfs.Root()
		require.NoError(t, err)
		root.ForgetAll()
		node, err = vfs.Stat(obj.Remote())
		require.NoError(t, err)
		return before, node.Inode()
	}

	t.Run("WithID", func(t *testing.T) {
		before, after := inodeAfterForget(t, newInoObject("file.txt", 4242))
		assert.Equal(t, uint64(4242), before)
		assert.Equal(t, before, after)
	})

	// Without a backend id there is nothing stable to derive the inode
	// from, so it changes.
	t.Run("WithoutID", func(t *testing.T) {
		before, after := inodeAfterForget(t, mockobject.New("file.txt").WithContent([]byte("x"), mockobject.SeekModeNone))
		assert.NotEqual(t, before, after)
	})
}
