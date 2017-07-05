// CodeMirror, copyright (c) by Marijn Haverbeke and others
// Distributed under an MIT license: http://codemirror.net/LICENSE

(function() {
  var mode = CodeMirror.getMode({indentUnit: 2}, "soy");
  function MT(name) {test.mode(name, mode, Array.prototype.slice.call(arguments, 1));}

  // Test of small keywords and words containing them.
  MT('keywords-test',
     '[keyword {] [keyword as] worrying [keyword and] notorious [keyword as]',
     '    the Fandor-alias assassin, [keyword or]',
     '    Corcand cannot fit [keyword in] [keyword }]');

  MT('let-test',
     '[keyword {template] [def .name][keyword }]',
     '  [keyword {let] [def $name]: [string "world"][keyword /}]',
     '  [tag&bracket <][tag h1][tag&bracket >]',
     '    Hello, [keyword {][variable-2 $name][keyword }]',
     '  [tag&bracket </][tag h1][tag&bracket >]',
     '[keyword {/template}]',
     '');

  MT('param-type-test',
     '[keyword {@param] [def a]: ' +
         '[variable-3 list]<[[[variable-3 a]: [variable-3 int], ' +
         '[variable-3 b]: [variable-3 map]<[variable-3 string], ' +
         '[variable-3 bool]>]]>][keyword }]');

  MT('undefined-var',
     '[keyword {][variable-2&error $var]');

  MT('param-scope-test',
     '[keyword {template] [def .a][keyword }]',
     '  [keyword {@param] [def x]: [variable-3 string][keyword }]',
     '  [keyword {][variable-2 $x][keyword }]',
     '[keyword {/template}]',
     '',
     '[keyword {template] [def .b][keyword }]',
     '  [keyword {][variable-2&error $x][keyword }]',
     '[keyword {/template}]',
     '');

  MT('if-variable-test',
     '[keyword {if] [variable-2&error $showThing][keyword }]',
     '  Yo!',
     '[keyword {/if}]',
     '');

  MT('defined-if-variable-test',
     '[keyword {template] [def .foo][keyword }]',
     '  [keyword {@param?] [def showThing]: [variable-3 bool][keyword }]',
     '  [keyword {if] [variable-2 $showThing][keyword }]',
     '    Yo!',
     '  [keyword {/if}]',
     '[keyword {/template}]',
     '');

  MT('template-calls-test',
     '[keyword {template] [def .foo][keyword }]',
     '  Yo!',
     '[keyword {/template}]',
     '[keyword {call] [variable-2 .foo][keyword /}]',
     '[keyword {call] [variable foo][keyword /}]',
     '[keyword {call] [variable .bar][keyword /}]',
     '[keyword {call] [variable bar][keyword /}]',
     '');

  MT('foreach-scope-test',
     '[keyword {@param] [def bar]: [variable-3 string][keyword }]',
     '[keyword {foreach] [def $foo] [keyword in] [variable-2&error $foos][keyword }]',
     '  [keyword {][variable-2 $foo][keyword }]',
     '[keyword {/foreach}]',
     '[keyword {][variable-2&error $foo][keyword }]',
     '[keyword {][variable-2 $bar][keyword }]');

  MT('foreach-ifempty-indent-test',
     '[keyword {foreach] [def $foo] [keyword in] [variable-2&error $foos][keyword }]',
     '  something',
     '[keyword {ifempty}]',
     '  nothing',
     '[keyword {/foreach}]',
     '');

  MT('nested-kind-test',
     '[keyword {template] [def .foo] [attribute kind]=[string "html"][keyword }]',
     '  [tag&bracket <][tag div][tag&bracket >]',
     '    [keyword {call] [variable .bar][keyword }]',
     '      [keyword {param] [attribute kind]=[string "js"][keyword }]',
     '        [keyword var] [def bar] [operator =] [number 5];',
     '      [keyword {/param}]',
     '    [keyword {/call}]',
     '  [tag&bracket </][tag div][tag&bracket >]',
     '[keyword {/template}]',
     '');

  MT('tag-starting-with-function-call-is-not-a-keyword',
     '[keyword {]index([variable-2&error $foo])[keyword }]',
     '[keyword {css] [string "some-class"][keyword }]',
     '[keyword {]css([string "some-class"])[keyword }]',
     '');

  MT('allow-missing-colon-in-@param',
     '[keyword {template] [def .foo][keyword }]',
     '  [keyword {@param] [def showThing] [variable-3 bool][keyword }]',
     '  [keyword {if] [variable-2 $showThing][keyword }]',
     '    Yo!',
     '  [keyword {/if}]',
     '[keyword {/template}]',
     '');

  MT('single-quote-strings',
     '[keyword {][string "foo"] [string \'bar\'][keyword }]',
     '');
})();
