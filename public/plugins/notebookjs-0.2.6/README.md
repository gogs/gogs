# notebook.js `v0.2.6`

Notebook.js parses raw [IPython](http://ipython.org/)/[Jupyter](http://jupyter.org/) notebooks, and lets you render them as HTML. See a __[working demo here](https://jsvine.github.io/nbpreview/)__.

## Usage

Notebook.js works in the browser and in Node.js. Usage is fairly straightforward.

### Browser Usage

First, provide access to `nb` via a script tag:

```html
<script src="notebook.js"></script>
```

Then parse, render, and (perhaps) append:

```
var notebook = nb.parse(raw_ipynb_json_string);
var rendered = notebook.render();
document.body.appendChild(rendered);
```

### IO.js Usage

*Note: To take advantage of `jsdom`'s latest features/bugfixes, `notebook.js` now runs on [io.js](https://iojs.org/) instead of Node.js.*

To install:

```sh
npm install notebookjs
```

Then parse, render, and write:

```js
var fs = require ("fs");
var nb = require("notebookjs");
var ipynb = JSON.parse(fs.readFileSync("path/to/notebook.ipynb"));
var notebook = nb.parse(ipynb);
console.log(notebook.render().outerHTML);
```

## Markdown and ANSI-coloring

By default, notebook.js supports [marked](https://github.com/chjj/marked) for Markdown rendering, and [ansi_up](https://github.com/drudru/ansi_up) for ANSI-coloring. It does not, however, ship with those libraries, so you must `<script>`-include or `require` them before initializing notebook.js.

To support other Markdown or ANSI-coloring engines, set `nb.markdown` and/or `nb.ansi` to functions that accept raw text and return rendered text.

## Code-Highlighting

Notebook.js plays well with code-highlighting libraries. See [NBPreview](https://github.com/jsvine/nbpreview) for an example of how to add support for your preferred highlighter.

## MathJax 

Notebook.js currently doesn't support MathJax. Implementation suggestions welcome. (Markdown-parsing was interfering with prior attempts.)

## Styling Rendered Notebooks

The HTML rendered by notebook.js (intentionally) does not contain any styling. But each key element has fairly straightfoward CSS classes that make styling your notebooks a cinch. See [NBPreview](https://github.com/jsvine/nbpreview/css) for an example implementation.

## Thanks

Many thanks to the following users for catching bugs, fixing typos, and proposing useful features:

- [@bradhowes](https://github.com/bradhowes)
- [@HavenZhang](https://github.com/HavenZhang)
