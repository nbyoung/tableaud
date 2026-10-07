package web

// What the tests of package web_test reach: the unexported seam of Render.
var (
	Parse    = parse
	LevelOf  = levelOf
	EnvOf    = envOf
	LegendOf = legendOf
	Adapters = adapters
)

// OpenAll is the opener of a Part.
type OpenAll = openAll
