package api

// The workbench's two fragment endpoints.
//
// THEY RETURN HTML, NOT JSON, and that is a decision rather than a shortcut.
// The island needs a connection's schemas and a relation's columns drawn into
// the tree; asking for JSON would mean the tree's classes, its ARIA and its
// "this warehouse does not say" exist twice -- once in templ, where the Go
// suite tests them, and once in a JavaScript string, where nothing would.
//
// So the server draws, as it does everywhere else here, and the script's
// whole job is to put the answer where the reload used to put it.
//
// GET AND NOT POST, because neither carries a statement. A target in a URL is
// already allowed -- `/data/target?u=` does it -- and a statement is the
// thing that must never be.
