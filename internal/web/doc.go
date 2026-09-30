// Package web holds what the daemon serves to a browser: the HTML templates
// and the static assets, embedded so that one binary carries them.
//
// The server in package main renders every view through Templates and mounts
// Static under /static/. The templates take view data from tablo and never
// read a task file.
package web
