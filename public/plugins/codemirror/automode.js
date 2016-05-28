CodeMirror.modeURL = "../mode/%N/%N.js";
var editor = CodeMirror.fromTextArea(document.getElementById("edit-area"), {
  lineNumbers: true
});
var filenameInput = document.getElementById("file-name");
CodeMirror.on(filenameInput, "change", function(e) {
  change();
});
function change() {
  var val = filenameInput.value, m, mode, spec;
  if (m = /.+\.([^.]+)$/.exec(val)) {
    var info = CodeMirror.findModeByExtension(m[1]);
    if (info) {
      mode = info.mode;
      spec = info.mime;
    }
  } else if (/\//.test(val)) {
    var info = CodeMirror.findModeByMIME(val);
    if (info) {
      mode = info.mode;
      spec = val;
    }
  } else {
    mode = spec = val;
  }
  if (mode) {
    editor.setOption("mode", spec);
    CodeMirror.autoLoadMode(editor, mode);
  } 
}

