// Package web holds what the daemon and the static export serve to a browser:
// the ten views, the validated parameters of a request, the page a template
// receives, and the HTML templates and static assets, embedded so that one
// binary carries them.
//
// A request reaches a template as a Page. Render executes the frame in
// templates/base.html around the view's own file under templates/views/, which
// a template task replaces. A template never writes an address: it asks the
// Page, which asks a Linker. The daemon's Linker, in package serve, spells a
// path and a query; the export's spells a file path.
//
// What stands inside <main> is a model: one Go type for each view, built by an
// adapter from the data of tablo and the Page, and drawn by a template that
// holds markup and fixed wording alone. A view with no adapter draws a
// placeholder.
//
// The HTTP side lives in package serve, and the command that wires it in
// cmd/tableaud. Package web imports source for the data of a view and never
// reads a task file.
package web
