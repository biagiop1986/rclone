//go:build linux || (darwin && amd64)

package mount2

import (
	"context"
	"path"
	"strings"
	"syscall"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// The kernel reconnects a filehandle whose inode it has evicted by asking
// two questions about a bare nodeid: LOOKUP "." for the node itself, and
// LOOKUP ".." for its parent, repeated up to the export root. Answering
// both from the backend is all that is required - in particular there is no
// need to walk a path or reattach the ancestors, since the kernel asks for
// each ancestor in turn.
//
// A node reached this way has no parent edge in go-fuse's tree, so
// fs.Inode.Path() cannot walk from it to the root. That costs nothing here:
// the VFS node knows its own path independently.

var (
	_ fusefs.NodeLookupNoder    = (*Node)(nil)
	_ fusefs.NodeLookupParenter = (*Node)(nil)
)

// enableNFSExport negotiates FUSE_EXPORT_SUPPORT, which makes the kernel ask
// us to resolve bare nodeids instead of failing a stale filehandle outright.
func enableNFSExport(o *fuse.MountOptions) {
	o.ExtraCapabilities |= fuse.CAP_EXPORT_SUPPORT
}

// LookupNode resolves a bare nodeid, which under ExternalNodeID is the
// inode number, back to its node.
//
// Only nodes in the VFS directory cache can be found. One that has been
// evicted is ESTALE, and a restart empties the cache entirely, so nothing
// resolves cold until ordinary traffic has repopulated it. Closing that gap
// needs the backend to resolve one of its own ids without help, which is a
// backend capability rather than something the VFS can synthesise.
func (n *Node) LookupNode(ctx context.Context, id uint64, out *fuse.EntryOut) (*fusefs.Inode, syscall.Errno) {
	found := n.fsys.VFS.FindByInode(id)
	if found == nil {
		return nil, syscall.ESTALE
	}
	n.fsys.setEntryOut(found, out)
	return n.NewInode(ctx, newNode(n.fsys, found), fusefs.StableAttr{
		Mode: out.Attr.Mode,
		Ino:  found.Inode(),
	}), 0
}

// LookupParent returns this node's parent directory and the name this node
// is listed under there.
//
// The VFS records each node's own path, so the parent is a plain lookup
// rather than a walk.
func (n *Node) LookupParent(ctx context.Context, out *fuse.EntryOut) (*fusefs.Inode, string, syscall.Errno) {
	p := strings.Trim(n.node.Path(), "/")
	if p == "" {
		return nil, "", syscall.ENOENT // the mount root has no parent here
	}
	parentPath, name := path.Split(p)

	parent, err := n.fsys.VFS.Stat(strings.TrimSuffix(parentPath, "/"))
	if err != nil {
		return nil, "", translateError(err)
	}
	n.fsys.setEntryOut(parent, out)
	return n.NewInode(ctx, newNode(n.fsys, parent), fusefs.StableAttr{
		Mode: out.Attr.Mode,
		Ino:  parent.Inode(),
	}), name, 0
}
