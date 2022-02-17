(function () {
  /* eslint-disable no-console */
  // From https://github.com/egoist/vue-ga/blob/master/src/index.js
  function appendScript() {
    var script = document.createElement('script');
    script.async = true;
    script.src = 'https://www.google-analytics.com/analytics.js';
    document.body.appendChild(script);
  }

  function init(id) {
    appendScript();
    window.ga =
      window.ga ||
      function() {
        (window.ga.q = window.ga.q || []).push(arguments);
      };

    window.ga.l = Number(new Date());
    window.ga('create', id, 'auto');
  }

  function collect() {
    if (!window.ga) {
      init($docsify.ga);
    }

    window.ga('set', 'page', location.hash);
    window.ga('send', 'pageview');
  }

  var install = function(hook) {
    if (!$docsify.ga) {
      console.error('[Docsify] ga is required.');
      return;
    }

    hook.beforeEach(collect);
  };

  $docsify.plugins = [].concat(install, $docsify.plugins);

}());
