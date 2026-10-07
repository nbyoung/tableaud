package export

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
)

// manifestPath is the name of the manifest, the last file the export writes.
const manifestPath = "manifest.json"

// manifest states the bundle and its source: the generator versions, the ref
// as given, the commit with its author date, the trunk, the arrival every page
// freezes, and each file but the manifest with its size and SHA-256, sorted by
// the bytes of its path. The keys stand in a fixed order, the indent is two
// spaces, the file ends with one newline, and nothing in it comes from the
// clock or the host.
func manifest(o Options, proj source.Project, ix source.Index, files map[string][]byte, pages []page) []byte {
	byPath := make(map[string]page, len(pages))
	for _, p := range pages {
		byPath[p.Path] = p
	}
	names := make([]string, 0, len(files))
	for n := range files {
		if n != manifestPath {
			names = append(names, n)
		}
	}
	slices.Sort(names)

	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString(`  "schema": "tableaud-export/1",` + "\n")
	b.WriteString(`  "generator": { "tableaud": ` + q(o.Tableaud) + `, "tablo": ` + q(o.Tablo) + ` },` + "\n")
	b.WriteString(`  "ref": { "name": ` + q(o.Ref) + `, "commit": ` + q(proj.Commit) + `, "date": ` + q(ix.Date.Format(time.RFC3339)) + ` },` + "\n")
	b.WriteString(`  "trunk": { "name": ` + q(ix.Trunk) + `, "commit": ` + q(ix.TrunkCommit) + ` },` + "\n")
	b.WriteString(`  "arrival": { "role": "observer", "level": "glance" },` + "\n")
	if len(names) == 0 {
		b.WriteString(`  "files": []` + "\n")
	} else {
		b.WriteString(`  "files": [` + "\n")
	}
	for i, n := range names {
		sum := sha256.Sum256(files[n])
		b.WriteString(`    { "path": ` + q(n))
		if p, ok := byPath[n]; ok {
			b.WriteString(`, "view": ` + q(p.View))
			if p.Task != "" {
				b.WriteString(`, "task": ` + q(p.Task))
			}
			if p.Person != "" {
				b.WriteString(`, "person": ` + q(p.Person))
			}
		}
		b.WriteString(`, "bytes": ` + strconv.Itoa(len(files[n])) + `, "sha256": ` + q(hex.EncodeToString(sum[:])) + ` }`)
		if i < len(names)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	if len(names) > 0 {
		b.WriteString("  ]\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

// q writes a JSON string with &, < and > unescaped.
func q(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}
