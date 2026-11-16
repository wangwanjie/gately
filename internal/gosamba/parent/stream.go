// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）。

//go:build linux || darwin

package parent

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/vfs"
)

// streamFileInfo is the os.FileInfo we return for a synthetic named-stream Open.
// Size reflects the in-memory stream buffer; everything else is constant.
type streamFileInfo struct {
	name string
	size int64
}

func (s streamFileInfo) Name() string       { return s.name }
func (s streamFileInfo) Size() int64        { return s.size }
func (s streamFileInfo) Mode() fs.FileMode  { return 0 }
func (s streamFileInfo) ModTime() time.Time { return time.Now() }
func (s streamFileInfo) IsDir() bool        { return false }
func (s streamFileInfo) Sys() any           { return nil }

// afpInfoSize is the fixed size of the NT "AFP_AfpInfo" metadata stream
// (MS/Apple AFP_INFO_SIZE = 0x3c). macOS reads this stream during a copy and
// requires a well-formed blob — a short/empty read makes Finder abort the copy
// before writing file data. See Samba source3/include/MacExtensions.h.
const afpInfoSize = 60

// isAFPInfoStream reports whether a stream name is Apple's metadata stream.
func isAFPInfoStream(name string) bool { return strings.EqualFold(name, "AFP_AfpInfo") }

// synthAFPInfo builds an empty-but-valid 60-byte AFP_AfpInfo blob: signature
// "AFP\0", version 0x00000100 (big-endian), zeroed FinderInfo. Mirrors Samba's
// afpinfo_new + afpinfo_pack (vfs_fruit) for a freshly-created, not-yet-written
// metadata stream.
func synthAFPInfo() []byte {
	b := make([]byte, afpInfoSize)
	b[0], b[1], b[2], b[3] = 'A', 'F', 'P', 0x00    // afpi_Signature
	b[4], b[5], b[6], b[7] = 0x00, 0x00, 0x01, 0x00 // afpi_Version 0x00000100 (BE)
	return b
}

// splitStreamName parses an SMB CREATE name that may carry NTFS alternate-
// data-stream syntax `path[:streamname[:type]]` and returns the base path,
// the stream name (empty for the default $DATA stream), and ok=false if the
// stream type is something other than $DATA.
//
// macOS writes extended attributes via stream syntax such as
// `foo.txt:com.apple.metadata:_kMDItemUserTags:$DATA`. The stream name itself
// can contain colons, so the split is "first colon separates basename from
// stream-component, last colon (if present and the suffix is `$DATA`)
// separates stream-name from stream-type".
func splitStreamName(name string) (base, stream string, ok bool) {
	i := strings.IndexByte(name, ':')
	if i < 0 {
		return name, "", true
	}
	base = name[:i]
	rest := name[i+1:]
	if j := strings.LastIndexByte(rest, ':'); j >= 0 {
		if !strings.EqualFold(rest[j+1:], "$DATA") {
			return "", "", false
		}
		stream = rest[:j]
		return base, stream, true
	}
	return base, rest, true
}

// handleCreateNamedStream services a CREATE on `path:streamname[:$DATA]`. The
// base file must exist; stream contents are persisted as a Linux user xattr
// (user.gosamba.ads.<stream>) so they survive across handles. READ/WRITE/
// SET_INFO operate on an in-memory streamBuf seeded here from the xattr; the
// buffer is flushed back to the xattr (or removed on DeleteOnClose) at CLOSE.
func (d *Dispatcher) handleCreateNamedStream(rw io.ReadWriter, hdr smb2.Header, sess *Session, tree *Tree, req smb2.CreateRequest, baseName, streamName string) bool {
	// Named-stream writes are mutations; deny them on read-only shares.
	// Reads of existing streams on a read-only share are allowed: a plain
	// Open disposition with no write-access bits is still served below.
	if tree.Share.ReadOnly {
		const writeMask = smb2.AccessFileWriteData | smb2.AccessFileAppendData |
			smb2.AccessGenericWrite | smb2.AccessGenericAll |
			0x00000100 | // FILE_WRITE_ATTRIBUTES
			0x00010000 // DELETE
		if req.CreateDisposition != smb2.CreateDispositionOpen || req.DesiredAccess&writeMask != 0 {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
	}
	// Use normalization-insensitive resolution, like every other CREATE path.
	// Plain ResolveSecure misses a base file stored NFD-encoded (the norm on
	// macOS) when the client addresses it in NFC, so a stream on such a file
	// looked like "base does not exist" and the whole open failed.
	osPath, err := vfs.ResolveSecureNorm(tree.Share.Path, baseName, shareFoldsCase(tree))
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
		return true
	}
	// Gately 补丁：被屏蔽的路径按不存在处理，见 gately_guard.go
	if shareHidden(tree, osPath) {
		d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
		return true
	}
	// The base object must exist — but it may be a DIRECTORY. macOS opens named
	// streams on directories to carry Finder metadata (tags, labels, custom
	// icons): smb2fs_smb_get_create_options() in Apple's smbfs deliberately
	// omits FILE_DIRECTORY_FILE when a stream name is present, and
	// smbfs_vnop_setxattr() has no vnode-type guard at all — it only requires
	// that the share advertise named streams, which this server does. Answering
	// FILE_IS_A_DIRECTORY here made `xattr -w` (and so every Finder folder tag)
	// fail with EIO on any folder in the share.
	//
	// Nothing downstream needs a file handle: this function never opens the base
	// object, the stream helpers in xattr.go are path-based (Getxattr/Setxattr/
	// Removexattr, all valid on a directory), and CLOSE's delete-on-close for a
	// stream removes only the backing xattr — it returns before the unlink that
	// serves ordinary handles. Path containment is unaffected: it is enforced by
	// ResolveSecureNorm above, not by the vnode type.
	if _, statErr := os.Lstat(osPath); statErr != nil {
		// Map missing/permission/traversal to OBJECT_NAME_NOT_FOUND so the
		// client doesn't retry.
		d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
		return true
	}

	// Honour CreateDisposition (MS-SMB2 §3.3.5.9 / MS-FSCC): a named stream is
	// an object in its own right, so FILE_OPEN of a stream that was never
	// written must fail with STATUS_OBJECT_NAME_NOT_FOUND and FILE_CREATE of
	// one that already exists must fail with STATUS_OBJECT_NAME_COLLISION.
	// Accepting every disposition silently materialised an empty stream on any
	// probe, so clients could never tell "no such stream" from "empty stream".
	exists, existErr := streamExists(osPath, streamName)
	switch {
	case errors.Is(existErr, errXattrUnsupported):
		// The filesystem cannot store streams at all (FAT/exFAT, some network
		// mounts). Reporting NOT_FOUND for every open would break clients that
		// merely probe; stay permissive, exactly as before, and let CLOSE drop
		// the data.
		exists = true
	case existErr != nil:
		d.Log.Warn("stream existence check failed", "path", osPath, "stream", streamName, "err", existErr)
		d.respondError(rw, hdr, statusFromErr(existErr), sess)
		return true
	}
	// AFP_AfpInfo is always presented: macOS reads it during a copy and aborts
	// the copy on a failed/empty read, so we synthesize a valid blob below
	// rather than reporting the stream missing.
	if isAFPInfoStream(streamName) {
		exists = true
	}

	// truncatesStream reports whether a disposition replaces the stream's
	// contents rather than opening them (MS-FSCC: SUPERSEDE, OVERWRITE and
	// OVERWRITE_IF all start from an empty file).
	truncatesStream := func(disp uint32) bool {
		switch disp {
		case smb2.CreateDispositionSupersede,
			smb2.CreateDispositionOverwrite,
			smb2.CreateDispositionOverwriteIf:
			return true
		}
		return false
	}

	createAction := uint32(smb2.CreateActionOpened)
	if truncatesStream(req.CreateDisposition) && exists {
		createAction = smb2.CreateActionOverwritten
	}
	switch req.CreateDisposition {
	case smb2.CreateDispositionOpen, smb2.CreateDispositionOverwrite:
		if !exists {
			d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
			return true
		}
	case smb2.CreateDispositionCreate:
		if exists {
			d.respondError(rw, hdr, smb2.StatusObjectNameCollision, sess)
			return true
		}
		createAction = smb2.CreateActionCreated
	case smb2.CreateDispositionOpenIf, smb2.CreateDispositionOverwriteIf, smb2.CreateDispositionSupersede:
		if !exists {
			createAction = smb2.CreateActionCreated
		}
	default:
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	maxAccess := uint32(0x001F01FF) // FILE_ALL_ACCESS
	if tree.Share.ReadOnly {
		maxAccess = 0x001200A9 // FILE_GENERIC_READ | FILE_GENERIC_EXECUTE
	}
	open := &Open{
		Path:          osPath,
		Tree:          tree,
		IsStream:      true,
		StreamName:    streamName,
		GrantedAccess: maxAccess,
	}
	// Load any previously persisted stream content so READ returns it. A stream
	// that was never written (or a filesystem without xattr support) yields an
	// empty buffer; writes still accumulate and get flushed at CLOSE.
	if buf, rerr := readStreamXattr(osPath, streamName); rerr == nil {
		open.streamBuf = buf
	} else if !errors.Is(rerr, errXattrUnsupported) {
		d.Log.Warn("stream xattr read failed", "path", osPath, "stream", streamName, "err", rerr)
	}
	// A truncating disposition must start the stream empty. Without this a
	// short write over a longer existing stream leaves the old tail behind:
	// the buffer is loaded from the xattr, the write overwrites only its
	// prefix, and CLOSE flushes prefix+stale-tail back. Do this after the
	// load (which would otherwise re-populate it) and before the AFP_AfpInfo
	// fabrication, so a superseded AFP stream still gets a valid 60-byte blob.
	if truncatesStream(req.CreateDisposition) {
		open.streamBuf = nil
		open.streamWritten = true
	}
	// Apple's AFP_AfpInfo metadata stream must always present a valid 60-byte
	// blob. macOS reads it during a copy; returning 0 bytes makes Finder abort
	// before writing the file data. Fabricate one when nothing is stored yet —
	// flagged synthetic so a read-only open doesn't persist a spurious xattr.
	if isAFPInfoStream(streamName) && len(open.streamBuf) < afpInfoSize {
		open.streamBuf = synthAFPInfo()
		open.streamSynthetic = true
	}
	if _, err := rand.Read(open.FileID[:]); err != nil {
		d.respondError(rw, hdr, smb2.StatusInternalError, sess)
		return true
	}
	// Held from before publication until the response is built; see the
	// equivalent comment in handleCreate. A stream handle's response is built
	// out of open.streamBuf, which a teardown does not touch — but the handle
	// still has to be unreachable-or-held, never reachable-and-unheld, or the
	// rule stops being checkable.
	open.mu.Lock()
	defer open.mu.Unlock()
	if !sess.AddOpen(open) {
		// Torn down under this CREATE. A stream handle is an in-memory buffer
		// with no descriptor and no share-mode reservation (shareModeApplies
		// excludes streams), so nothing has to be given back.
		d.respondError(rw, hdr, smb2.StatusUserSessionDeleted, sess)
		return true
	}
	d.LastCreatedFileID = open.FileID
	d.HasLastCreated = true

	now := filetimeFromTime(time.Now())
	resp := smb2.EncodeCreateResponse(smb2.CreateResponse{
		CreateAction:   createAction,
		CreationTime:   now,
		LastAccessTime: now,
		LastWriteTime:  now,
		ChangeTime:     now,
		FileAttributes: smb2.FileAttrNormal,
		AllocationSize: uint64(len(open.streamBuf)),
		EndOfFile:      uint64(len(open.streamBuf)),
		FileID:         open.FileID,
		// Named-stream handles have no inode of their own, so pass a zero
		// disk-file-id: buildCreateResponseContexts then omits QFid entirely
		// instead of reporting an id of zero, which would make macOS drop
		// File-ID support for the rest of the session.
		CreateContexts: buildCreateResponseContexts(req.CreateContexts, d.Conn, tree, maxAccess, 0, 0),
	})
	d.respondSuccess(rw, hdr, sess, resp)
	return true
}
