// CodeMirror, copyright (c) by Marijn Haverbeke and others
// Distributed under an MIT license: http://codemirror.net/LICENSE

(function() {
  var mode = CodeMirror.getMode({indentUnit: 2}, "text/x-c");
  function MT(name) { test.mode(name, mode, Array.prototype.slice.call(arguments, 1)); }

  MT("indent",
     "[type void] [def foo]([type void*] [variable a], [type int] [variable b]) {",
     "  [type int] [variable c] [operator =] [variable b] [operator +]",
     "    [number 1];",
     "  [keyword return] [operator *][variable a];",
     "}");

  MT("indent_switch",
     "[keyword switch] ([variable x]) {",
     "  [keyword case] [number 10]:",
     "    [keyword return] [number 20];",
     "  [keyword default]:",
     "    [variable printf]([string \"foo %c\"], [variable x]);",
     "}");

  MT("def",
     "[type void] [def foo]() {}",
     "[keyword struct] [def bar]{}",
     "[type int] [type *][def baz]() {}");

  MT("def_new_line",
     "::[variable std]::[variable SomeTerribleType][operator <][variable T][operator >]",
     "[def SomeLongMethodNameThatDoesntFitIntoOneLine]([keyword const] [variable MyType][operator &] [variable param]) {}")

  MT("double_block",
     "[keyword for] (;;)",
     "  [keyword for] (;;)",
     "    [variable x][operator ++];",
     "[keyword return];");

  MT("preprocessor",
     "[meta #define FOO 3]",
     "[type int] [variable foo];",
     "[meta #define BAR\\]",
     "[meta 4]",
     "[type unsigned] [type int] [variable bar] [operator =] [number 8];",
     "[meta #include <baz> ][comment // comment]")


  var mode_cpp = CodeMirror.getMode({indentUnit: 2}, "text/x-c++src");
  function MTCPP(name) { test.mode(name, mode_cpp, Array.prototype.slice.call(arguments, 1)); }

  MTCPP("cpp14_literal",
    "[number 10'000];",
    "[number 0b10'000];",
    "[number 0x10'000];",
    "[string '100000'];");

  MTCPP("ctor_dtor",
     "[def Foo::Foo]() {}",
     "[def Foo::~Foo]() {}");
})();
