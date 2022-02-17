(function () {
  /* eslint-disable no-unused-vars */
  var fixedPath = location.href.replace('/-/', '/#/');
  if (fixedPath !== location.href) {
    location.href = fixedPath;
  }

  function install(hook, vm) {
    var dom = Docsify.dom;
    var disqus = vm.config.disqus;
    if (!disqus) {
      throw Error('$docsify.disqus is required');
    }

    hook.init(function (_) {
      var script = dom.create('script');

      script.async = true;
      script.src = "https://" + disqus + ".disqus.com/embed.js";
      script.setAttribute('data-timestamp', Number(new Date()));
      dom.appendTo(dom.body, script);
    });

    hook.mounted(function (_) {
      var div = dom.create('div');
      div.id = 'disqus_thread';
      var main = dom.getNode('#main');
      div.style = "width: " + (main.clientWidth) + "px; margin: 0 auto 20px;";
      dom.appendTo(dom.find('.content'), div);

      // eslint-disable-next-line
      window.disqus_config = function() {
        this.page.url = location.origin + '/-' + vm.route.path;
        this.page.identifier = vm.route.path;
        this.page.title = document.title;
      };
    });

    hook.doneEach(function (_) {
      if (typeof window.DISQUS !== 'undefined') {
        window.DISQUS.reset({
          reload: true,
          config: function() {
            this.page.url = location.origin + '/-' + vm.route.path;
            this.page.identifier = vm.route.path;
            this.page.title = document.title;
          },
        });
      }
    });
  }

  $docsify.plugins = [].concat(install, $docsify.plugins);

}());
