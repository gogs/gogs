(function () {
  function handleExternalScript() {
    var container = Docsify.dom.getNode('#main');
    var scripts = Docsify.dom.findAll(container, 'script');

    for (var i = scripts.length; i--; ) {
      var script = scripts[i];

      if (script && script.src) {
        var newScript = document.createElement('script');

        Array.prototype.slice.call(script.attributes).forEach(function (attribute) {
          newScript[attribute.name] = attribute.value;
        });

        script.parentNode.insertBefore(newScript, script);
        script.parentNode.removeChild(script);
      }
    }
  }

  var install = function(hook) {
    hook.doneEach(handleExternalScript);
  };

  window.$docsify.plugins = [].concat(install, window.$docsify.plugins);

}());
