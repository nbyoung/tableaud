// Package serve is the HTTP side of tableaud: one route per view, the query
// parser and writer, the daemon's Linker, the watcher that tells when the
// repository changed, the tag and the cache that spare tablo a call, and the
// Host check.
//
// A Server holds no state of the repository: every page derives from tablo's
// data through a source.Source, and every address a page holds comes from a
// web.Linker. The daemon never writes to the repository, binds only the
// address it is given, and sets no cookie.
package serve
