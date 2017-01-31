#!/usr/bin/env iojs
// Reads a notebook from stdin, prints the rendered HTML to stdout
var fs = require("fs");
var nb = require("../notebook.js");
var ipynb = JSON.parse(fs.readFileSync("/dev/stdin"));
var notebook = nb.parse(ipynb);
console.log(notebook.render().outerHTML);
