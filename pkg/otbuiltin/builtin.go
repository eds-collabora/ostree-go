// Package otbuiltin contains all of the basic commands for creating and
// interacting with an ostree repository
package otbuiltin

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	glib "github.com/ostreedev/ostree-go/pkg/glibobject"
)

// #cgo pkg-config: ostree-1
// #include <stdlib.h>
// #include <glib.h>
// #include <ostree.h>
// #include "builtin.go.h"
import "C"

// Repo represents a local ostree repository
type Repo struct {
	ptr unsafe.Pointer
}

func cCancellable(c *glib.GCancellable) *C.GCancellable {
	return (*C.GCancellable)(c.Ptr())
}

// isInitialized checks if the repo has been initialized
func (r *Repo) isInitialized() bool {
	if r == nil || r.ptr == nil {
		return false
	}
	return true
}

// native converts an ostree repo struct to its C equivalent
func (r *Repo) native() *C.OstreeRepo {
	if !r.isInitialized() {
		return nil
	}
	return (*C.OstreeRepo)(r.ptr)
}

// repoFromNative takes a C ostree repo and converts it to a Go struct
func repoFromNative(or *C.OstreeRepo) *Repo {
	if or == nil {
		return nil
	}
	r := &Repo{unsafe.Pointer(or)}
	return r
}

func (repo *Repo) ResolveRev(refspec string, allowNoent bool) (string, error) {
	var cerr *C.GError = nil
	var coutrev *C.char = nil

	crefspec := C.CString(refspec)
	defer C.free(unsafe.Pointer(crefspec))

	r := C.ostree_repo_resolve_rev(repo.native(), crefspec, (C.gboolean)(glib.GBool(allowNoent)), &coutrev, &cerr)
	if !gobool(r) {
		return "", generateError(cerr)
	}

	outrev := C.GoString(coutrev)
	C.free(unsafe.Pointer(coutrev))

	return outrev, nil
}

// OpenRepo attempts to open the repo at the given path
func OpenRepo(path string) (*Repo, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	repoPath := C.g_file_new_for_path(cpath)
	defer C.g_object_unref(C.gpointer(repoPath))
	crepo := C.ostree_repo_new(repoPath)
	repo := repoFromNative(crepo)

	var cerr *C.GError
	r := glib.GoBool(glib.GBoolean(C.ostree_repo_open(crepo, nil, &cerr)))
	if !r {
		return nil, generateError(cerr)
	}

	return repo, nil
}

type PullOptions struct {
	OverrideRemoteName string
	Refs               []string
}

func (repo *Repo) PullWithOptions(remoteName string, options PullOptions, progress *AsyncProgress, cancellable *glib.GCancellable) error {
	var cerr *C.GError = nil

	cremoteName := C.CString(remoteName)
	defer C.free(unsafe.Pointer(cremoteName))

	builder := C.g_variant_builder_new(C._g_variant_type(C.CString("a{sv}")))
	if options.OverrideRemoteName != "" {
		cstr := C.CString(options.OverrideRemoteName)
		v := C.g_variant_new_take_string((*C.gchar)(cstr))
		k := C.CString("override-remote-name")
		defer C.free(unsafe.Pointer(k))
		C._g_variant_builder_add_twoargs(builder, C.CString("{sv}"), k, v)
	}

	if len(options.Refs) != 0 {
		crefs := make([]*C.gchar, len(options.Refs))
		for i, s := range options.Refs {
			crefs[i] = (*C.gchar)(C.CString(s))
		}

		v := C.g_variant_new_strv((**C.gchar)(&crefs[0]), (C.gssize)(len(crefs)))

		for i, s := range crefs {
			crefs[i] = nil
			C.free(unsafe.Pointer(s))
		}

		k := C.CString("refs")
		defer C.free(unsafe.Pointer(k))

		C._g_variant_builder_add_twoargs(builder, C.CString("{sv}"), k, v)
	}

	coptions := C.g_variant_builder_end(builder)

	r := C.ostree_repo_pull_with_options(repo.native(), cremoteName, coptions, progress.native(), cCancellable(cancellable), &cerr)

	if !gobool(r) {
		return generateError(cerr)
	}

	return nil
}

// enableTombstoneCommits enables support for tombstone commits.
//
// This allows to distinguish between intentional deletions and accidental removals
// of commits.
func (r *Repo) enableTombstoneCommits() error {
	if !r.isInitialized() {
		return errors.New("repo not initialized")
	}

	config := C.ostree_repo_get_config(r.native())
	groupC := C.CString("core")
	defer C.free(unsafe.Pointer(groupC))
	keyC := C.CString("tombstone-commits")
	defer C.free(unsafe.Pointer(keyC))
	valueC := C.g_key_file_get_boolean(config, (*C.gchar)(groupC), (*C.gchar)(keyC), nil)
	tombstoneCommits := glib.GoBool(glib.GBoolean(valueC))

	// tombstoneCommits is false only if it really is false or if it is set to FALSE in the config file
	if !tombstoneCommits {
		var cerr *C.GError
		C.g_key_file_set_boolean(config, (*C.gchar)(groupC), (*C.gchar)(keyC), C.TRUE)
		if !glib.GoBool(glib.GBoolean(C.ostree_repo_write_config(r.native(), config, &cerr))) {
			return generateError(cerr)
		}
	}
	return nil
}

// generateError wraps a GLib error into a Go one.
func generateError(err *C.GError) error {
	if err == nil {
		return errors.New("nil GError")
	}

	goErr := glib.ConvertGError(glib.ToGError(unsafe.Pointer(err)))
	_, file, line, ok := runtime.Caller(1)
	if ok {
		return fmt.Errorf("%s:%d - %s", file, line, goErr)
	}
	return goErr
}

// isOk wraps a gboolean return value into a bool.
// 0 is false/error, everything else is true/ok.
func isOk(v C.gboolean) bool {
	return glib.GoBool(glib.GBoolean(v))
}

func gobool(b C.gboolean) bool {
	return b != C.FALSE
}

type AsyncProgress struct {
	*glib.Object
}

func (a *AsyncProgress) native() *C.OstreeAsyncProgress {
	if a == nil {
		return nil
	}
	return (*C.OstreeAsyncProgress)(a.Ptr())
}
