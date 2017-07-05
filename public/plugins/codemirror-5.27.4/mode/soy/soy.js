// CodeMirror, copyright (c) by Marijn Haverbeke and others
// Distributed under an MIT license: http://codemirror.net/LICENSE

(function(mod) {
  if (typeof exports == "object" && typeof module == "object") // CommonJS
    mod(require("../../lib/codemirror"), require("../htmlmixed/htmlmixed"));
  else if (typeof define == "function" && define.amd) // AMD
    define(["../../lib/codemirror", "../htmlmixed/htmlmixed"], mod);
  else // Plain browser env
    mod(CodeMirror);
})(function(CodeMirror) {
  "use strict";

  var indentingTags = ["template", "literal", "msg", "fallbackmsg", "let", "if", "elseif",
                       "else", "switch", "case", "default", "foreach", "ifempty", "for",
                       "call", "param", "deltemplate", "delcall", "log"];

  CodeMirror.defineMode("soy", function(config) {
    var textMode = CodeMirror.getMode(config, "text/plain");
    var modes = {
      html: CodeMirror.getMode(config, {name: "text/html", multilineTagIndentFactor: 2, multilineTagIndentPastTag: false}),
      attributes: textMode,
      text: textMode,
      uri: textMode,
      css: CodeMirror.getMode(config, "text/css"),
      js: CodeMirror.getMode(config, {name: "text/javascript", statementIndent: 2 * config.indentUnit})
    };

    function last(array) {
      return array[array.length - 1];
    }

    function tokenUntil(stream, state, untilRegExp) {
      if (stream.sol()) {
        for (var indent = 0; indent < state.indent; indent++) {
          if (!stream.eat(/\s/)) break;
        }
        if (indent) return null;
      }
      var oldString = stream.string;
      var match = untilRegExp.exec(oldString.substr(stream.pos));
      if (match) {
        // We don't use backUp because it backs up just the position, not the state.
        // This uses an undocumented API.
        stream.string = oldString.substr(0, stream.pos + match.index);
      }
      var result = stream.hideFirstChars(state.indent, function() {
        var localState = last(state.localStates);
        return localState.mode.token(stream, localState.state);
      });
      stream.string = oldString;
      return result;
    }

    function contains(list, element) {
      while (list) {
        if (list.element === element) return true;
        list = list.next;
      }
      return false;
    }

    function prepend(list, element) {
      return {
        element: element,
        next: list
      };
    }

    // Reference a variable `name` in `list`.
    // Let `loose` be truthy to ignore missing identifiers.
    function ref(list, name, loose) {
      return contains(list, name) ? "variable-2" : (loose ? "variable" : "variable-2 error");
    }

    function popscope(state) {
      if (state.scopes) {
        state.variables = state.scopes.element;
        state.scopes = state.scopes.next;
      }
    }

    return {
      startState: function() {
        return {
          kind: [],
          kindTag: [],
          soyState: [],
          templates: null,
          variables: null,
          scopes: null,
          indent: 0,
          quoteKind: null,
          localStates: [{
            mode: modes.html,
            state: CodeMirror.startState(modes.html)
          }]
        };
      },

      copyState: function(state) {
        return {
          tag: state.tag, // Last seen Soy tag.
          kind: state.kind.concat([]), // Values of kind="" attributes.
          kindTag: state.kindTag.concat([]), // Opened tags with kind="" attributes.
          soyState: state.soyState.concat([]),
          templates: state.templates,
          variables: state.variables,
          scopes: state.scopes,
          indent: state.indent, // Indentation of the following line.
          quoteKind: state.quoteKind,
          localStates: state.localStates.map(function(localState) {
            return {
              mode: localState.mode,
              state: CodeMirror.copyState(localState.mode, localState.state)
            };
          })
        };
      },

      token: function(stream, state) {
        var match;

        switch (last(state.soyState)) {
          case "comment":
            if (stream.match(/^.*?\*\//)) {
              state.soyState.pop();
            } else {
              stream.skipToEnd();
            }
            return "comment";

          case "templ-def":
            if (match = stream.match(/^\.?([\w]+(?!\.[\w]+)*)/)) {
              state.templates = prepend(state.templates, match[1]);
              state.scopes = prepend(state.scopes, state.variables);
              state.soyState.pop();
              return "def";
            }
            stream.next();
            return null;

          case "templ-ref":
            if (match = stream.match(/^\.?([\w]+)/)) {
              state.soyState.pop();
              // If the first character is '.', try to match against a local template name.
              if (match[0][0] == '.') {
                return ref(state.templates, match[1], true);
              }
              // Otherwise
              return "variable";
            }
            stream.next();
            return null;

          case "param-def":
            if (match = stream.match(/^\w+/)) {
              state.variables = prepend(state.variables, match[0]);
              state.soyState.pop();
              state.soyState.push("param-type");
              return "def";
            }
            stream.next();
            return null;

          case "param-type":
            if (stream.peek() == "}") {
              state.soyState.pop();
              return null;
            }
            if (stream.eatWhile(/^[\w]+/)) {
              return "variable-3";
            }
            stream.next();
            return null;

          case "var-def":
            if (match = stream.match(/^\$([\w]+)/)) {
              state.variables = prepend(state.variables, match[1]);
              state.soyState.pop();
              return "def";
            }
            stream.next();
            return null;

          case "tag":
            if (stream.match(/^\/?}/)) {
              if (state.tag == "/template" || state.tag == "/deltemplate") {
                popscope(state);
                state.indent = 0;
              } else {
                if (state.tag == "/for" || state.tag == "/foreach") {
                  popscope(state);
                }
                state.indent -= config.indentUnit *
                    (stream.current() == "/}" || indentingTags.indexOf(state.tag) == -1 ? 2 : 1);
              }
              state.soyState.pop();
              return "keyword";
            } else if (stream.match(/^([\w?]+)(?==)/)) {
              if (stream.current() == "kind" && (match = stream.match(/^="([^"]+)/, false))) {
                var kind = match[1];
                state.kind.push(kind);
                state.kindTag.push(state.tag);
                var mode = modes[kind] || modes.html;
                var localState = last(state.localStates);
                if (localState.mode.indent) {
                  state.indent += localState.mode.indent(localState.state, "");
                }
                state.localStates.push({
                  mode: mode,
                  state: CodeMirror.startState(mode)
                });
              }
              return "attribute";
            } else if (match = stream.match(/^["']/)) {
              state.soyState.push("string");
              state.quoteKind = match;
              return "string";
            }
            if (match = stream.match(/^\$([\w]+)/)) {
              return ref(state.variables, match[1]);
            }
            if (match = stream.match(/^\w+/)) {
              return /^(?:as|and|or|not|in)$/.test(match[0]) ? "keyword" : null;
            }
            stream.next();
            return null;

          case "literal":
            if (stream.match(/^(?=\{\/literal})/)) {
              state.indent -= config.indentUnit;
              state.soyState.pop();
              return this.token(stream, state);
            }
            return tokenUntil(stream, state, /\{\/literal}/);

          case "string":
            var match = stream.match(/^.*?(["']|\\[\s\S])/);
            if (!match) {
              stream.skipToEnd();
            } else if (match[1] == state.quoteKind) {
              state.quoteKind = null;
              state.soyState.pop();
            }
            return "string";
        }

        if (stream.match(/^\/\*/)) {
          state.soyState.push("comment");
          return "comment";
        } else if (stream.match(stream.sol() ? /^\s*\/\/.*/ : /^\s+\/\/.*/)) {
          return "comment";
        } else if (stream.match(/^\{literal}/)) {
          state.indent += config.indentUnit;
          state.soyState.push("literal");
          return "keyword";

        // A tag-keyword must be followed by whitespace or a closing tag.
        } else if (match = stream.match(/^\{([\/@\\]?\w+\??)(?=[\s\}])/)) {
          if (match[1] != "/switch")
            state.indent += (/^(\/|(else|elseif|ifempty|case|fallbackmsg|default)$)/.test(match[1]) && state.tag != "switch" ? 1 : 2) * config.indentUnit;
          state.tag = match[1];
          if (state.tag == "/" + last(state.kindTag)) {
            // We found the tag that opened the current kind="".
            state.kind.pop();
            state.kindTag.pop();
            state.localStates.pop();
            var localState = last(state.localStates);
            if (localState.mode.indent) {
              state.indent -= localState.mode.indent(localState.state, "");
            }
          }
          state.soyState.push("tag");
          if (state.tag == "template" || state.tag == "deltemplate") {
            state.soyState.push("templ-def");
          }
          if (state.tag == "call" || state.tag == "delcall") {
            state.soyState.push("templ-ref");
          }
          if (state.tag == "let") {
            state.soyState.push("var-def");
          }
          if (state.tag == "for" || state.tag == "foreach") {
            state.scopes = prepend(state.scopes, state.variables);
            state.soyState.push("var-def");
          }
          if (state.tag.match(/^@(?:param\??|inject)/)) {
            state.soyState.push("param-def");
          }
          return "keyword";

        // Not a tag-keyword; it's an implicit print tag.
        } else if (stream.eat('{')) {
          state.tag = "print";
          state.indent += 2 * config.indentUnit;
          state.soyState.push("tag");
          return "keyword";
        }

        return tokenUntil(stream, state, /\{|\s+\/\/|\/\*/);
      },

      indent: function(state, textAfter) {
        var indent = state.indent, top = last(state.soyState);
        if (top == "comment") return CodeMirror.Pass;

        if (top == "literal") {
          if (/^\{\/literal}/.test(textAfter)) indent -= config.indentUnit;
        } else {
          if (/^\s*\{\/(template|deltemplate)\b/.test(textAfter)) return 0;
          if (/^\{(\/|(fallbackmsg|elseif|else|ifempty)\b)/.test(textAfter)) indent -= config.indentUnit;
          if (state.tag != "switch" && /^\{(case|default)\b/.test(textAfter)) indent -= config.indentUnit;
          if (/^\{\/switch\b/.test(textAfter)) indent -= config.indentUnit;
        }
        var localState = last(state.localStates);
        if (indent && localState.mode.indent) {
          indent += localState.mode.indent(localState.state, textAfter);
        }
        return indent;
      },

      innerMode: function(state) {
        if (state.soyState.length && last(state.soyState) != "literal") return null;
        else return last(state.localStates);
      },

      electricInput: /^\s*\{(\/|\/template|\/deltemplate|\/switch|fallbackmsg|elseif|else|case|default|ifempty|\/literal\})$/,
      lineComment: "//",
      blockCommentStart: "/*",
      blockCommentEnd: "*/",
      blockCommentContinue: " * ",
      useInnerComments: false,
      fold: "indent"
    };
  }, "htmlmixed");

  CodeMirror.registerHelper("hintWords", "soy", indentingTags.concat(
      ["delpackage", "namespace", "alias", "print", "css", "debugger"]));

  CodeMirror.defineMIME("text/x-soy", "soy");
});
