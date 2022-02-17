(function () {
  /**
   * Create a cached version of a pure function.
   * @param {*} fn The function call to be cached
   * @void
   */

  function cached(fn) {
    var cache = Object.create(null);
    return function(str) {
      var key = isPrimitive(str) ? str : JSON.stringify(str);
      var hit = cache[key];
      return hit || (cache[key] = fn(str));
    };
  }

  /**
   * Hyphenate a camelCase string.
   */
  var hyphenate = cached(function (str) {
    return str.replace(/([A-Z])/g, function (m) { return '-' + m.toLowerCase(); });
  });

  var hasOwn = Object.prototype.hasOwnProperty;

  /**
   * Simple Object.assign polyfill
   * @param {Object} to The object to be merged with
   * @returns {Object} The merged object
   */
  var merge =
    Object.assign ||
    function(to) {
      var arguments$1 = arguments;

      for (var i = 1; i < arguments.length; i++) {
        var from = Object(arguments$1[i]);

        for (var key in from) {
          if (hasOwn.call(from, key)) {
            to[key] = from[key];
          }
        }
      }

      return to;
    };

  /**
   * Check if value is primitive
   * @param {*} value Checks if a value is primitive
   * @returns {Boolean} Result of the check
   */
  function isPrimitive(value) {
    return typeof value === 'string' || typeof value === 'number';
  }

  /**
   * Performs no operation.
   * @void
   */
  function noop() {}

  /**
   * Check if value is function
   * @param {*} obj Any javascript object
   * @returns {Boolean} True if the passed-in value is a function
   */
  function isFn(obj) {
    return typeof obj === 'function';
  }

  /**
   * Check if url is external
   * @param {String} string  url
   * @returns {Boolean} True if the passed-in url is external
   */
  function isExternal(url) {
    var match = url.match(
      /^([^:/?#]+:)?(?:\/{2,}([^/?#]*))?([^?#]+)?(\?[^#]*)?(#.*)?/
    );

    if (
      typeof match[1] === 'string' &&
      match[1].length > 0 &&
      match[1].toLowerCase() !== location.protocol
    ) {
      return true;
    }
    if (
      typeof match[2] === 'string' &&
      match[2].length > 0 &&
      match[2].replace(
        new RegExp(
          ':(' + { 'http:': 80, 'https:': 443 }[location.protocol] + ')?$'
        ),
        ''
      ) !== location.host
    ) {
      return true;
    }
    return false;
  }

  var inBrowser = !false;

  var isMobile =  document.body.clientWidth <= 600;

  /**
   * @see https://github.com/MoOx/pjax/blob/master/lib/is-supported.js
   */
  var supportsPushState =
    
    (function() {
      // Borrowed wholesale from https://github.com/defunkt/jquery-pjax
      return (
        window.history &&
        window.history.pushState &&
        window.history.replaceState &&
        // PushState isn’t reliable on iOS until 5.
        !navigator.userAgent.match(
          /((iPod|iPhone|iPad).+\bOS\s+[1-4]\D|WebApps\/.+CFNetwork)/
        )
      );
    })();

  var cacheNode = {};

  /**
   * Get Node
   * @param  {String|Element} el A DOM element
   * @param  {Boolean} noCache Flag to use or not use the cache
   * @return {Element} The found node element
   */
  function getNode(el, noCache) {
    if ( noCache === void 0 ) noCache = false;

    if (typeof el === 'string') {
      if (typeof window.Vue !== 'undefined') {
        return find(el);
      }

      el = noCache ? find(el) : cacheNode[el] || (cacheNode[el] = find(el));
    }

    return el;
  }

  var $ =  document;

  var body =  $.body;

  var head =  $.head;

  /**
   * Find elements
   * @param {String|Element} el The root element where to perform the search from
   * @param {Element} node The query
   * @returns {Element} The found DOM element
   * @example
   * find('nav') => document.querySelector('nav')
   * find(nav, 'a') => nav.querySelector('a')
   */
  function find(el, node) {
    return node ? el.querySelector(node) : $.querySelector(el);
  }

  /**
   * Find all elements
   * @param {String|Element} el The root element where to perform the search from
   * @param {Element} node The query
   * @returns {Array<Element>} An array of DOM elements
   * @example
   * findAll('a') => [].slice.call(document.querySelectorAll('a'))
   * findAll(nav, 'a') => [].slice.call(nav.querySelectorAll('a'))
   */
  function findAll(el, node) {
    return [].slice.call(
      node ? el.querySelectorAll(node) : $.querySelectorAll(el)
    );
  }

  function create(node, tpl) {
    node = $.createElement(node);
    if (tpl) {
      node.innerHTML = tpl;
    }

    return node;
  }

  function appendTo(target, el) {
    return target.appendChild(el);
  }

  function before(target, el) {
    return target.insertBefore(el, target.children[0]);
  }

  function on(el, type, handler) {
    isFn(type)
      ? window.addEventListener(el, type)
      : el.addEventListener(type, handler);
  }

  function off(el, type, handler) {
    isFn(type)
      ? window.removeEventListener(el, type)
      : el.removeEventListener(type, handler);
  }

  /**
   * Toggle class
   * @param {String|Element} el The element that needs the class to be toggled
   * @param {Element} type The type of action to be performed on the classList (toggle by default)
   * @param {String} val Name of the class to be toggled
   * @void
   * @example
   * toggleClass(el, 'active') => el.classList.toggle('active')
   * toggleClass(el, 'add', 'active') => el.classList.add('active')
   */
  function toggleClass(el, type, val) {
    el && el.classList[val ? type : 'toggle'](val || type);
  }

  function style(content) {
    appendTo(head, create('style', content));
  }

  /**
   * Fork https://github.com/bendrucker/document-ready/blob/master/index.js
   * @param {Function} callback The callbacack to be called when the page is loaded
   * @returns {Number|void} If the page is already laoded returns the result of the setTimeout callback,
   *  otherwise it only attaches the callback to the DOMContentLoaded event
   */
  function documentReady(callback, doc) {
    if ( doc === void 0 ) doc = document;

    var state = doc.readyState;

    if (state === 'complete' || state === 'interactive') {
      return setTimeout(callback, 0);
    }

    doc.addEventListener('DOMContentLoaded', callback);
  }

  var dom = /*#__PURE__*/Object.freeze({
    __proto__: null,
    getNode: getNode,
    $: $,
    body: body,
    head: head,
    find: find,
    findAll: findAll,
    create: create,
    appendTo: appendTo,
    before: before,
    on: on,
    off: off,
    toggleClass: toggleClass,
    style: style,
    documentReady: documentReady
  });

  var decode = decodeURIComponent;
  var encode = encodeURIComponent;

  function parseQuery(query) {
    var res = {};

    query = query.trim().replace(/^(\?|#|&)/, '');

    if (!query) {
      return res;
    }

    // Simple parse
    query.split('&').forEach(function(param) {
      var parts = param.replace(/\+/g, ' ').split('=');

      res[parts[0]] = parts[1] && decode(parts[1]);
    });

    return res;
  }

  function stringifyQuery(obj, ignores) {
    if ( ignores === void 0 ) ignores = [];

    var qs = [];

    for (var key in obj) {
      if (ignores.indexOf(key) > -1) {
        continue;
      }

      qs.push(
        obj[key]
          ? ((encode(key)) + "=" + (encode(obj[key]))).toLowerCase()
          : encode(key)
      );
    }

    return qs.length ? ("?" + (qs.join('&'))) : '';
  }

  var isAbsolutePath = cached(function (path) {
    return /(:|(\/{2}))/g.test(path);
  });

  var removeParams = cached(function (path) {
    return path.split(/[?#]/)[0];
  });

  var getParentPath = cached(function (path) {
    if (/\/$/g.test(path)) {
      return path;
    }

    var matchingParts = path.match(/(\S*\/)[^/]+$/);
    return matchingParts ? matchingParts[1] : '';
  });

  var cleanPath = cached(function (path) {
    return path.replace(/^\/+/, '/').replace(/([^:])\/{2,}/g, '$1/');
  });

  var resolvePath = cached(function (path) {
    var segments = path.replace(/^\//, '').split('/');
    var resolved = [];
    for (var i = 0, len = segments.length; i < len; i++) {
      var segment = segments[i];
      if (segment === '..') {
        resolved.pop();
      } else if (segment !== '.') {
        resolved.push(segment);
      }
    }

    return '/' + resolved.join('/');
  });

  /**
   * Normalises the URI path to handle the case where Docsify is
   * hosted off explicit files, i.e. /index.html. This function
   * eliminates any path segments that contain `#` fragments.
   *
   * This is used to map browser URIs to markdown file sources.
   *
   * For example:
   *
   * http://example.org/base/index.html#/blah
   *
   * would be mapped to:
   *
   * http://example.org/base/blah.md.
   *
   * See here for more information:
   *
   * https://github.com/docsifyjs/docsify/pull/1372
   *
   * @param {string} path The URI path to normalise
   * @return {string} { path, query }
   */

  function normaliseFragment(path) {
    return path
      .split('/')
      .filter(function (p) { return p.indexOf('#') === -1; })
      .join('/');
  }

  function getPath() {
    var args = [], len = arguments.length;
    while ( len-- ) args[ len ] = arguments[ len ];

    return cleanPath(args.map(normaliseFragment).join('/'));
  }

  var replaceSlug = cached(function (path) {
    return path.replace('#', '?id=');
  });

  function endsWith(str, suffix) {
    return str.indexOf(suffix, str.length - suffix.length) !== -1;
  }

  var cached$1 = {};

  function getAlias(path, alias, last) {
    var match = Object.keys(alias).filter(function (key) {
      var re = cached$1[key] || (cached$1[key] = new RegExp(("^" + key + "$")));
      return re.test(path) && path !== last;
    })[0];

    return match
      ? getAlias(path.replace(cached$1[match], alias[match]), alias, path)
      : path;
  }

  function getFileName(path, ext) {
    return new RegExp(("\\.(" + (ext.replace(/^\./, '')) + "|html)$"), 'g').test(path)
      ? path
      : /\/$/g.test(path)
      ? (path + "README" + ext)
      : ("" + path + ext);
  }

  var History = function History(config) {
    this.config = config;
  };

  History.prototype.getBasePath = function getBasePath () {
    return this.config.basePath;
  };

  History.prototype.getFile = function getFile (path, isRelative) {
      if ( path === void 0 ) path = this.getCurrentPath();

    var ref = this;
      var config = ref.config;
    var base = this.getBasePath();
    var ext = typeof config.ext === 'string' ? config.ext : '.md';

    path = config.alias ? getAlias(path, config.alias) : path;
    path = getFileName(path, ext);
    path = path === ("/README" + ext) ? config.homepage || path : path;
    path = isAbsolutePath(path) ? path : getPath(base, path);

    if (isRelative) {
      path = path.replace(new RegExp(("^" + base)), '');
    }

    return path;
  };

  History.prototype.onchange = function onchange (cb) {
      if ( cb === void 0 ) cb = noop;

    cb();
  };

  History.prototype.getCurrentPath = function getCurrentPath () {};

  History.prototype.normalize = function normalize () {};

  History.prototype.parse = function parse () {};

  History.prototype.toURL = function toURL (path, params, currentRoute) {
    var local = currentRoute && path[0] === '#';
    var route = this.parse(replaceSlug(path));

    route.query = merge({}, route.query, params);
    path = route.path + stringifyQuery(route.query);
    path = path.replace(/\.md(\?)|\.md$/, '$1');

    if (local) {
      var idIndex = currentRoute.indexOf('?');
      path =
        (idIndex > 0 ? currentRoute.substring(0, idIndex) : currentRoute) +
        path;
    }

    if (this.config.relativePath && path.indexOf('/') !== 0) {
      var currentDir = currentRoute.substring(
        0,
        currentRoute.lastIndexOf('/') + 1
      );
      return cleanPath(resolvePath(currentDir + path));
    }

    return cleanPath('/' + path);
  };

  function replaceHash(path) {
    var i = location.href.indexOf('#');
    location.replace(location.href.slice(0, i >= 0 ? i : 0) + '#' + path);
  }
  var HashHistory = /*@__PURE__*/(function (History) {
    function HashHistory(config) {
      History.call(this, config);
      this.mode = 'hash';
    }

    if ( History ) HashHistory.__proto__ = History;
    HashHistory.prototype = Object.create( History && History.prototype );
    HashHistory.prototype.constructor = HashHistory;

    HashHistory.prototype.getBasePath = function getBasePath () {
      var path = window.location.pathname || '';
      var base = this.config.basePath;

      // This handles the case where Docsify is served off an
      // explicit file path, i.e.`/base/index.html#/blah`. This
      // prevents the `/index.html` part of the URI from being
      // remove during routing.
      // See here: https://github.com/docsifyjs/docsify/pull/1372
      var basePath = endsWith(path, '.html')
        ? path + '#/' + base
        : path + '/' + base;
      return /^(\/|https?:)/g.test(base) ? base : cleanPath(basePath);
    };

    HashHistory.prototype.getCurrentPath = function getCurrentPath () {
      // We can't use location.hash here because it's not
      // consistent across browsers - Firefox will pre-decode it!
      var href = location.href;
      var index = href.indexOf('#');
      return index === -1 ? '' : href.slice(index + 1);
    };

    /** @param {((params: {source: TODO}) => void)} [cb] */
    HashHistory.prototype.onchange = function onchange (cb) {
      if ( cb === void 0 ) cb = noop;

      // The hashchange event does not tell us if it originated from
      // a clicked link or by moving back/forward in the history;
      // therefore we set a `navigating` flag when a link is clicked
      // to be able to tell these two scenarios apart
      var navigating = false;

      on('click', function (e) {
        var el = e.target.tagName === 'A' ? e.target : e.target.parentNode;

        if (el && el.tagName === 'A' && !/_blank/.test(el.target)) {
          navigating = true;
        }
      });

      on('hashchange', function (e) {
        var source = navigating ? 'navigate' : 'history';
        navigating = false;
        cb({ event: e, source: source });
      });
    };

    HashHistory.prototype.normalize = function normalize () {
      var path = this.getCurrentPath();

      path = replaceSlug(path);

      if (path.charAt(0) === '/') {
        return replaceHash(path);
      }

      replaceHash('/' + path);
    };

    /**
     * Parse the url
     * @param {string} [path=location.herf] URL to be parsed
     * @return {object} { path, query }
     */
    HashHistory.prototype.parse = function parse (path) {
      if ( path === void 0 ) path = location.href;

      var query = '';

      var hashIndex = path.indexOf('#');
      if (hashIndex >= 0) {
        path = path.slice(hashIndex + 1);
      }

      var queryIndex = path.indexOf('?');
      if (queryIndex >= 0) {
        query = path.slice(queryIndex + 1);
        path = path.slice(0, queryIndex);
      }

      return {
        path: path,
        file: this.getFile(path, true),
        query: parseQuery(query),
      };
    };

    HashHistory.prototype.toURL = function toURL (path, params, currentRoute) {
      return '#' + History.prototype.toURL.call(this, path, params, currentRoute);
    };

    return HashHistory;
  }(History));

  /** @typedef {any} TODO */

  var HTML5History = /*@__PURE__*/(function (History) {
    function HTML5History(config) {
      History.call(this, config);
      this.mode = 'history';
    }

    if ( History ) HTML5History.__proto__ = History;
    HTML5History.prototype = Object.create( History && History.prototype );
    HTML5History.prototype.constructor = HTML5History;

    HTML5History.prototype.getCurrentPath = function getCurrentPath () {
      var base = this.getBasePath();
      var path = window.location.pathname;

      if (base && path.indexOf(base) === 0) {
        path = path.slice(base.length);
      }

      return (path || '/') + window.location.search + window.location.hash;
    };

    HTML5History.prototype.onchange = function onchange (cb) {
      var this$1 = this;
      if ( cb === void 0 ) cb = noop;

      on('click', function (e) {
        var el = e.target.tagName === 'A' ? e.target : e.target.parentNode;

        if (el && el.tagName === 'A' && !/_blank/.test(el.target)) {
          e.preventDefault();
          var url = el.href;
          // solve history.pushState cross-origin issue
          if (this$1.config.crossOriginLinks.indexOf(url) !== -1) {
            window.open(url, '_self');
          } else {
            window.history.pushState({ key: url }, '', url);
          }
          cb({ event: e, source: 'navigate' });
        }
      });

      on('popstate', function (e) {
        cb({ event: e, source: 'history' });
      });
    };

    /**
     * Parse the url
     * @param {string} [path=location.href] URL to be parsed
     * @return {object} { path, query }
     */
    HTML5History.prototype.parse = function parse (path) {
      if ( path === void 0 ) path = location.href;

      var query = '';

      var queryIndex = path.indexOf('?');
      if (queryIndex >= 0) {
        query = path.slice(queryIndex + 1);
        path = path.slice(0, queryIndex);
      }

      var base = getPath(location.origin);
      var baseIndex = path.indexOf(base);

      if (baseIndex > -1) {
        path = path.slice(baseIndex + base.length);
      }

      return {
        path: path,
        file: this.getFile(path),
        query: parseQuery(query),
      };
    };

    return HTML5History;
  }(History));

  /**
   * @typedef {{
   *   path?: string
   * }} Route
   */

  /** @type {Route} */
  var lastRoute = {};

  /** @typedef {import('../Docsify').Constructor} Constructor */

  /**
   * @template {!Constructor} T
   * @param {T} Base - The class to extend
   */
  function Router(Base) {
    return /*@__PURE__*/(function (Base) {
      function Router() {
        var args = [], len = arguments.length;
        while ( len-- ) args[ len ] = arguments[ len ];

        Base.apply(this, args);

        this.route = {};
      }

      if ( Base ) Router.__proto__ = Base;
      Router.prototype = Object.create( Base && Base.prototype );
      Router.prototype.constructor = Router;

      Router.prototype.updateRender = function updateRender () {
        this.router.normalize();
        this.route = this.router.parse();
        body.setAttribute('data-page', this.route.file);
      };

      Router.prototype.initRouter = function initRouter () {
        var this$1 = this;

        var config = this.config;
        var mode = config.routerMode || 'hash';
        var router;

        if (mode === 'history' && supportsPushState) {
          router = new HTML5History(config);
        } else {
          router = new HashHistory(config);
        }

        this.router = router;
        this.updateRender();
        lastRoute = this.route;

        // eslint-disable-next-line no-unused-vars
        router.onchange(function (params) {
          this$1.updateRender();
          this$1._updateRender();

          if (lastRoute.path === this$1.route.path) {
            this$1.$resetEvents(params.source);
            return;
          }

          this$1.$fetch(noop, this$1.$resetEvents.bind(this$1, params.source));
          lastRoute = this$1.route;
        });
      };

      return Router;
    }(Base));
  }

  var RGX = /([^{]*?)\w(?=\})/g;

  var MAP = {
  	YYYY: 'getFullYear',
  	YY: 'getYear',
  	MM: function (d) {
  		return d.getMonth() + 1;
  	},
  	DD: 'getDate',
  	HH: 'getHours',
  	mm: 'getMinutes',
  	ss: 'getSeconds',
  	fff: 'getMilliseconds'
  };

  function tinydate (str, custom) {
  	var parts=[], offset=0;

  	str.replace(RGX, function (key, _, idx) {
  		// save preceding string
  		parts.push(str.substring(offset, idx - 1));
  		offset = idx += key.length + 1;
  		// save function
  		parts.push(custom && custom[key] || function (d) {
  			return ('00' + (typeof MAP[key] === 'string' ? d[MAP[key]]() : MAP[key](d))).slice(-key.length);
  		});
  	});

  	if (offset !== str.length) {
  		parts.push(str.substring(offset));
  	}

  	return function (arg) {
  		var out='', i=0, d=arg||new Date();
  		for (; i<parts.length; i++) {
  			out += (typeof parts[i]==='string') ? parts[i] : parts[i](d);
  		}
  		return out;
  	};
  }

  /*! @license DOMPurify 2.3.1 | (c) Cure53 and other contributors | Released under the Apache license 2.0 and Mozilla Public License 2.0 | github.com/cure53/DOMPurify/blob/2.3.1/LICENSE */

  function _toConsumableArray(arr) { if (Array.isArray(arr)) { for (var i = 0, arr2 = Array(arr.length); i < arr.length; i++) { arr2[i] = arr[i]; } return arr2; } else { return Array.from(arr); } }

  var hasOwnProperty = Object.hasOwnProperty,
      setPrototypeOf = Object.setPrototypeOf,
      isFrozen = Object.isFrozen,
      getPrototypeOf = Object.getPrototypeOf,
      getOwnPropertyDescriptor = Object.getOwnPropertyDescriptor;
  var freeze = Object.freeze,
      seal = Object.seal,
      create$1 = Object.create; // eslint-disable-line import/no-mutable-exports

  var _ref = typeof Reflect !== 'undefined' && Reflect,
      apply = _ref.apply,
      construct = _ref.construct;

  if (!apply) {
    apply = function apply(fun, thisValue, args) {
      return fun.apply(thisValue, args);
    };
  }

  if (!freeze) {
    freeze = function freeze(x) {
      return x;
    };
  }

  if (!seal) {
    seal = function seal(x) {
      return x;
    };
  }

  if (!construct) {
    construct = function construct(Func, args) {
      return new (Function.prototype.bind.apply(Func, [null].concat(_toConsumableArray(args))))();
    };
  }

  var arrayForEach = unapply(Array.prototype.forEach);
  var arrayPop = unapply(Array.prototype.pop);
  var arrayPush = unapply(Array.prototype.push);

  var stringToLowerCase = unapply(String.prototype.toLowerCase);
  var stringMatch = unapply(String.prototype.match);
  var stringReplace = unapply(String.prototype.replace);
  var stringIndexOf = unapply(String.prototype.indexOf);
  var stringTrim = unapply(String.prototype.trim);

  var regExpTest = unapply(RegExp.prototype.test);

  var typeErrorCreate = unconstruct(TypeError);

  function unapply(func) {
    return function (thisArg) {
      var arguments$1 = arguments;

      for (var _len = arguments.length, args = Array(_len > 1 ? _len - 1 : 0), _key = 1; _key < _len; _key++) {
        args[_key - 1] = arguments$1[_key];
      }

      return apply(func, thisArg, args);
    };
  }

  function unconstruct(func) {
    return function () {
      var arguments$1 = arguments;

      for (var _len2 = arguments.length, args = Array(_len2), _key2 = 0; _key2 < _len2; _key2++) {
        args[_key2] = arguments$1[_key2];
      }

      return construct(func, args);
    };
  }

  /* Add properties to a lookup table */
  function addToSet(set, array) {
    if (setPrototypeOf) {
      // Make 'in' and truthy checks like Boolean(set.constructor)
      // independent of any properties defined on Object.prototype.
      // Prevent prototype setters from intercepting set as a this value.
      setPrototypeOf(set, null);
    }

    var l = array.length;
    while (l--) {
      var element = array[l];
      if (typeof element === 'string') {
        var lcElement = stringToLowerCase(element);
        if (lcElement !== element) {
          // Config presets (e.g. tags.js, attrs.js) are immutable.
          if (!isFrozen(array)) {
            array[l] = lcElement;
          }

          element = lcElement;
        }
      }

      set[element] = true;
    }

    return set;
  }

  /* Shallow clone an object */
  function clone(object) {
    var newObject = create$1(null);

    var property = void 0;
    for (property in object) {
      if (apply(hasOwnProperty, object, [property])) {
        newObject[property] = object[property];
      }
    }

    return newObject;
  }

  /* IE10 doesn't support __lookupGetter__ so lets'
   * simulate it. It also automatically checks
   * if the prop is function or getter and behaves
   * accordingly. */
  function lookupGetter(object, prop) {
    while (object !== null) {
      var desc = getOwnPropertyDescriptor(object, prop);
      if (desc) {
        if (desc.get) {
          return unapply(desc.get);
        }

        if (typeof desc.value === 'function') {
          return unapply(desc.value);
        }
      }

      object = getPrototypeOf(object);
    }

    function fallbackValue(element) {
      console.warn('fallback value for', element);
      return null;
    }

    return fallbackValue;
  }

  var html = freeze(['a', 'abbr', 'acronym', 'address', 'area', 'article', 'aside', 'audio', 'b', 'bdi', 'bdo', 'big', 'blink', 'blockquote', 'body', 'br', 'button', 'canvas', 'caption', 'center', 'cite', 'code', 'col', 'colgroup', 'content', 'data', 'datalist', 'dd', 'decorator', 'del', 'details', 'dfn', 'dialog', 'dir', 'div', 'dl', 'dt', 'element', 'em', 'fieldset', 'figcaption', 'figure', 'font', 'footer', 'form', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'head', 'header', 'hgroup', 'hr', 'html', 'i', 'img', 'input', 'ins', 'kbd', 'label', 'legend', 'li', 'main', 'map', 'mark', 'marquee', 'menu', 'menuitem', 'meter', 'nav', 'nobr', 'ol', 'optgroup', 'option', 'output', 'p', 'picture', 'pre', 'progress', 'q', 'rp', 'rt', 'ruby', 's', 'samp', 'section', 'select', 'shadow', 'small', 'source', 'spacer', 'span', 'strike', 'strong', 'style', 'sub', 'summary', 'sup', 'table', 'tbody', 'td', 'template', 'textarea', 'tfoot', 'th', 'thead', 'time', 'tr', 'track', 'tt', 'u', 'ul', 'var', 'video', 'wbr']);

  // SVG
  var svg = freeze(['svg', 'a', 'altglyph', 'altglyphdef', 'altglyphitem', 'animatecolor', 'animatemotion', 'animatetransform', 'circle', 'clippath', 'defs', 'desc', 'ellipse', 'filter', 'font', 'g', 'glyph', 'glyphref', 'hkern', 'image', 'line', 'lineargradient', 'marker', 'mask', 'metadata', 'mpath', 'path', 'pattern', 'polygon', 'polyline', 'radialgradient', 'rect', 'stop', 'style', 'switch', 'symbol', 'text', 'textpath', 'title', 'tref', 'tspan', 'view', 'vkern']);

  var svgFilters = freeze(['feBlend', 'feColorMatrix', 'feComponentTransfer', 'feComposite', 'feConvolveMatrix', 'feDiffuseLighting', 'feDisplacementMap', 'feDistantLight', 'feFlood', 'feFuncA', 'feFuncB', 'feFuncG', 'feFuncR', 'feGaussianBlur', 'feMerge', 'feMergeNode', 'feMorphology', 'feOffset', 'fePointLight', 'feSpecularLighting', 'feSpotLight', 'feTile', 'feTurbulence']);

  // List of SVG elements that are disallowed by default.
  // We still need to know them so that we can do namespace
  // checks properly in case one wants to add them to
  // allow-list.
  var svgDisallowed = freeze(['animate', 'color-profile', 'cursor', 'discard', 'fedropshadow', 'feimage', 'font-face', 'font-face-format', 'font-face-name', 'font-face-src', 'font-face-uri', 'foreignobject', 'hatch', 'hatchpath', 'mesh', 'meshgradient', 'meshpatch', 'meshrow', 'missing-glyph', 'script', 'set', 'solidcolor', 'unknown', 'use']);

  var mathMl = freeze(['math', 'menclose', 'merror', 'mfenced', 'mfrac', 'mglyph', 'mi', 'mlabeledtr', 'mmultiscripts', 'mn', 'mo', 'mover', 'mpadded', 'mphantom', 'mroot', 'mrow', 'ms', 'mspace', 'msqrt', 'mstyle', 'msub', 'msup', 'msubsup', 'mtable', 'mtd', 'mtext', 'mtr', 'munder', 'munderover']);

  // Similarly to SVG, we want to know all MathML elements,
  // even those that we disallow by default.
  var mathMlDisallowed = freeze(['maction', 'maligngroup', 'malignmark', 'mlongdiv', 'mscarries', 'mscarry', 'msgroup', 'mstack', 'msline', 'msrow', 'semantics', 'annotation', 'annotation-xml', 'mprescripts', 'none']);

  var text = freeze(['#text']);

  var html$1 = freeze(['accept', 'action', 'align', 'alt', 'autocapitalize', 'autocomplete', 'autopictureinpicture', 'autoplay', 'background', 'bgcolor', 'border', 'capture', 'cellpadding', 'cellspacing', 'checked', 'cite', 'class', 'clear', 'color', 'cols', 'colspan', 'controls', 'controlslist', 'coords', 'crossorigin', 'datetime', 'decoding', 'default', 'dir', 'disabled', 'disablepictureinpicture', 'disableremoteplayback', 'download', 'draggable', 'enctype', 'enterkeyhint', 'face', 'for', 'headers', 'height', 'hidden', 'high', 'href', 'hreflang', 'id', 'inputmode', 'integrity', 'ismap', 'kind', 'label', 'lang', 'list', 'loading', 'loop', 'low', 'max', 'maxlength', 'media', 'method', 'min', 'minlength', 'multiple', 'muted', 'name', 'noshade', 'novalidate', 'nowrap', 'open', 'optimum', 'pattern', 'placeholder', 'playsinline', 'poster', 'preload', 'pubdate', 'radiogroup', 'readonly', 'rel', 'required', 'rev', 'reversed', 'role', 'rows', 'rowspan', 'spellcheck', 'scope', 'selected', 'shape', 'size', 'sizes', 'span', 'srclang', 'start', 'src', 'srcset', 'step', 'style', 'summary', 'tabindex', 'title', 'translate', 'type', 'usemap', 'valign', 'value', 'width', 'xmlns', 'slot']);

  var svg$1 = freeze(['accent-height', 'accumulate', 'additive', 'alignment-baseline', 'ascent', 'attributename', 'attributetype', 'azimuth', 'basefrequency', 'baseline-shift', 'begin', 'bias', 'by', 'class', 'clip', 'clippathunits', 'clip-path', 'clip-rule', 'color', 'color-interpolation', 'color-interpolation-filters', 'color-profile', 'color-rendering', 'cx', 'cy', 'd', 'dx', 'dy', 'diffuseconstant', 'direction', 'display', 'divisor', 'dur', 'edgemode', 'elevation', 'end', 'fill', 'fill-opacity', 'fill-rule', 'filter', 'filterunits', 'flood-color', 'flood-opacity', 'font-family', 'font-size', 'font-size-adjust', 'font-stretch', 'font-style', 'font-variant', 'font-weight', 'fx', 'fy', 'g1', 'g2', 'glyph-name', 'glyphref', 'gradientunits', 'gradienttransform', 'height', 'href', 'id', 'image-rendering', 'in', 'in2', 'k', 'k1', 'k2', 'k3', 'k4', 'kerning', 'keypoints', 'keysplines', 'keytimes', 'lang', 'lengthadjust', 'letter-spacing', 'kernelmatrix', 'kernelunitlength', 'lighting-color', 'local', 'marker-end', 'marker-mid', 'marker-start', 'markerheight', 'markerunits', 'markerwidth', 'maskcontentunits', 'maskunits', 'max', 'mask', 'media', 'method', 'mode', 'min', 'name', 'numoctaves', 'offset', 'operator', 'opacity', 'order', 'orient', 'orientation', 'origin', 'overflow', 'paint-order', 'path', 'pathlength', 'patterncontentunits', 'patterntransform', 'patternunits', 'points', 'preservealpha', 'preserveaspectratio', 'primitiveunits', 'r', 'rx', 'ry', 'radius', 'refx', 'refy', 'repeatcount', 'repeatdur', 'restart', 'result', 'rotate', 'scale', 'seed', 'shape-rendering', 'specularconstant', 'specularexponent', 'spreadmethod', 'startoffset', 'stddeviation', 'stitchtiles', 'stop-color', 'stop-opacity', 'stroke-dasharray', 'stroke-dashoffset', 'stroke-linecap', 'stroke-linejoin', 'stroke-miterlimit', 'stroke-opacity', 'stroke', 'stroke-width', 'style', 'surfacescale', 'systemlanguage', 'tabindex', 'targetx', 'targety', 'transform', 'text-anchor', 'text-decoration', 'text-rendering', 'textlength', 'type', 'u1', 'u2', 'unicode', 'values', 'viewbox', 'visibility', 'version', 'vert-adv-y', 'vert-origin-x', 'vert-origin-y', 'width', 'word-spacing', 'wrap', 'writing-mode', 'xchannelselector', 'ychannelselector', 'x', 'x1', 'x2', 'xmlns', 'y', 'y1', 'y2', 'z', 'zoomandpan']);

  var mathMl$1 = freeze(['accent', 'accentunder', 'align', 'bevelled', 'close', 'columnsalign', 'columnlines', 'columnspan', 'denomalign', 'depth', 'dir', 'display', 'displaystyle', 'encoding', 'fence', 'frame', 'height', 'href', 'id', 'largeop', 'length', 'linethickness', 'lspace', 'lquote', 'mathbackground', 'mathcolor', 'mathsize', 'mathvariant', 'maxsize', 'minsize', 'movablelimits', 'notation', 'numalign', 'open', 'rowalign', 'rowlines', 'rowspacing', 'rowspan', 'rspace', 'rquote', 'scriptlevel', 'scriptminsize', 'scriptsizemultiplier', 'selection', 'separator', 'separators', 'stretchy', 'subscriptshift', 'supscriptshift', 'symmetric', 'voffset', 'width', 'xmlns']);

  var xml = freeze(['xlink:href', 'xml:id', 'xlink:title', 'xml:space', 'xmlns:xlink']);

  // eslint-disable-next-line unicorn/better-regex
  var MUSTACHE_EXPR = seal(/\{\{[\s\S]*|[\s\S]*\}\}/gm); // Specify template detection regex for SAFE_FOR_TEMPLATES mode
  var ERB_EXPR = seal(/<%[\s\S]*|[\s\S]*%>/gm);
  var DATA_ATTR = seal(/^data-[\-\w.\u00B7-\uFFFF]/); // eslint-disable-line no-useless-escape
  var ARIA_ATTR = seal(/^aria-[\-\w]+$/); // eslint-disable-line no-useless-escape
  var IS_ALLOWED_URI = seal(/^(?:(?:(?:f|ht)tps?|mailto|tel|callto|cid|xmpp):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i // eslint-disable-line no-useless-escape
  );
  var IS_SCRIPT_OR_DATA = seal(/^(?:\w+script|data):/i);
  var ATTR_WHITESPACE = seal(/[\u0000-\u0020\u00A0\u1680\u180E\u2000-\u2029\u205F\u3000]/g // eslint-disable-line no-control-regex
  );

  var _typeof = typeof Symbol === "function" && typeof Symbol.iterator === "symbol" ? function (obj) { return typeof obj; } : function (obj) { return obj && typeof Symbol === "function" && obj.constructor === Symbol && obj !== Symbol.prototype ? "symbol" : typeof obj; };

  function _toConsumableArray$1(arr) { if (Array.isArray(arr)) { for (var i = 0, arr2 = Array(arr.length); i < arr.length; i++) { arr2[i] = arr[i]; } return arr2; } else { return Array.from(arr); } }

  var getGlobal = function getGlobal() {
    return typeof window === 'undefined' ? null : window;
  };

  /**
   * Creates a no-op policy for internal use only.
   * Don't export this function outside this module!
   * @param {?TrustedTypePolicyFactory} trustedTypes The policy factory.
   * @param {Document} document The document object (to determine policy name suffix)
   * @return {?TrustedTypePolicy} The policy created (or null, if Trusted Types
   * are not supported).
   */
  var _createTrustedTypesPolicy = function _createTrustedTypesPolicy(trustedTypes, document) {
    if ((typeof trustedTypes === 'undefined' ? 'undefined' : _typeof(trustedTypes)) !== 'object' || typeof trustedTypes.createPolicy !== 'function') {
      return null;
    }

    // Allow the callers to control the unique policy name
    // by adding a data-tt-policy-suffix to the script element with the DOMPurify.
    // Policy creation with duplicate names throws in Trusted Types.
    var suffix = null;
    var ATTR_NAME = 'data-tt-policy-suffix';
    if (document.currentScript && document.currentScript.hasAttribute(ATTR_NAME)) {
      suffix = document.currentScript.getAttribute(ATTR_NAME);
    }

    var policyName = 'dompurify' + (suffix ? '#' + suffix : '');

    try {
      return trustedTypes.createPolicy(policyName, {
        createHTML: function createHTML(html$$1) {
          return html$$1;
        }
      });
    } catch (_) {
      // Policy creation failed (most likely another DOMPurify script has
      // already run). Skip creating the policy, as this will only cause errors
      // if TT are enforced.
      console.warn('TrustedTypes policy ' + policyName + ' could not be created.');
      return null;
    }
  };

  function createDOMPurify() {
    var window = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : getGlobal();

    var DOMPurify = function DOMPurify(root) {
      return createDOMPurify(root);
    };

    /**
     * Version label, exposed for easier checks
     * if DOMPurify is up to date or not
     */
    DOMPurify.version = '2.3.1';

    /**
     * Array of elements that DOMPurify removed during sanitation.
     * Empty if nothing was removed.
     */
    DOMPurify.removed = [];

    if (!window || !window.document || window.document.nodeType !== 9) {
      // Not running in a browser, provide a factory function
      // so that you can pass your own Window
      DOMPurify.isSupported = false;

      return DOMPurify;
    }

    var originalDocument = window.document;

    var document = window.document;
    var DocumentFragment = window.DocumentFragment,
        HTMLTemplateElement = window.HTMLTemplateElement,
        Node = window.Node,
        Element = window.Element,
        NodeFilter = window.NodeFilter,
        _window$NamedNodeMap = window.NamedNodeMap,
        NamedNodeMap = _window$NamedNodeMap === undefined ? window.NamedNodeMap || window.MozNamedAttrMap : _window$NamedNodeMap,
        Text = window.Text,
        Comment = window.Comment,
        DOMParser = window.DOMParser,
        trustedTypes = window.trustedTypes;


    var ElementPrototype = Element.prototype;

    var cloneNode = lookupGetter(ElementPrototype, 'cloneNode');
    var getNextSibling = lookupGetter(ElementPrototype, 'nextSibling');
    var getChildNodes = lookupGetter(ElementPrototype, 'childNodes');
    var getParentNode = lookupGetter(ElementPrototype, 'parentNode');

    // As per issue #47, the web-components registry is inherited by a
    // new document created via createHTMLDocument. As per the spec
    // (http://w3c.github.io/webcomponents/spec/custom/#creating-and-passing-registries)
    // a new empty registry is used when creating a template contents owner
    // document, so we use that as our parent document to ensure nothing
    // is inherited.
    if (typeof HTMLTemplateElement === 'function') {
      var template = document.createElement('template');
      if (template.content && template.content.ownerDocument) {
        document = template.content.ownerDocument;
      }
    }

    var trustedTypesPolicy = _createTrustedTypesPolicy(trustedTypes, originalDocument);
    var emptyHTML = trustedTypesPolicy && RETURN_TRUSTED_TYPE ? trustedTypesPolicy.createHTML('') : '';

    var _document = document,
        implementation = _document.implementation,
        createNodeIterator = _document.createNodeIterator,
        createDocumentFragment = _document.createDocumentFragment,
        getElementsByTagName = _document.getElementsByTagName;
    var importNode = originalDocument.importNode;


    var documentMode = {};
    try {
      documentMode = clone(document).documentMode ? document.documentMode : {};
    } catch (_) {}

    var hooks = {};

    /**
     * Expose whether this browser supports running the full DOMPurify.
     */
    DOMPurify.isSupported = typeof getParentNode === 'function' && implementation && typeof implementation.createHTMLDocument !== 'undefined' && documentMode !== 9;

    var MUSTACHE_EXPR$$1 = MUSTACHE_EXPR,
        ERB_EXPR$$1 = ERB_EXPR,
        DATA_ATTR$$1 = DATA_ATTR,
        ARIA_ATTR$$1 = ARIA_ATTR,
        IS_SCRIPT_OR_DATA$$1 = IS_SCRIPT_OR_DATA,
        ATTR_WHITESPACE$$1 = ATTR_WHITESPACE;
    var IS_ALLOWED_URI$$1 = IS_ALLOWED_URI;

    /**
     * We consider the elements and attributes below to be safe. Ideally
     * don't add any new ones but feel free to remove unwanted ones.
     */

    /* allowed element names */

    var ALLOWED_TAGS = null;
    var DEFAULT_ALLOWED_TAGS = addToSet({}, [].concat(_toConsumableArray$1(html), _toConsumableArray$1(svg), _toConsumableArray$1(svgFilters), _toConsumableArray$1(mathMl), _toConsumableArray$1(text)));

    /* Allowed attribute names */
    var ALLOWED_ATTR = null;
    var DEFAULT_ALLOWED_ATTR = addToSet({}, [].concat(_toConsumableArray$1(html$1), _toConsumableArray$1(svg$1), _toConsumableArray$1(mathMl$1), _toConsumableArray$1(xml)));

    /* Explicitly forbidden tags (overrides ALLOWED_TAGS/ADD_TAGS) */
    var FORBID_TAGS = null;

    /* Explicitly forbidden attributes (overrides ALLOWED_ATTR/ADD_ATTR) */
    var FORBID_ATTR = null;

    /* Decide if ARIA attributes are okay */
    var ALLOW_ARIA_ATTR = true;

    /* Decide if custom data attributes are okay */
    var ALLOW_DATA_ATTR = true;

    /* Decide if unknown protocols are okay */
    var ALLOW_UNKNOWN_PROTOCOLS = false;

    /* Output should be safe for common template engines.
     * This means, DOMPurify removes data attributes, mustaches and ERB
     */
    var SAFE_FOR_TEMPLATES = false;

    /* Decide if document with <html>... should be returned */
    var WHOLE_DOCUMENT = false;

    /* Track whether config is already set on this instance of DOMPurify. */
    var SET_CONFIG = false;

    /* Decide if all elements (e.g. style, script) must be children of
     * document.body. By default, browsers might move them to document.head */
    var FORCE_BODY = false;

    /* Decide if a DOM `HTMLBodyElement` should be returned, instead of a html
     * string (or a TrustedHTML object if Trusted Types are supported).
     * If `WHOLE_DOCUMENT` is enabled a `HTMLHtmlElement` will be returned instead
     */
    var RETURN_DOM = false;

    /* Decide if a DOM `DocumentFragment` should be returned, instead of a html
     * string  (or a TrustedHTML object if Trusted Types are supported) */
    var RETURN_DOM_FRAGMENT = false;

    /* If `RETURN_DOM` or `RETURN_DOM_FRAGMENT` is enabled, decide if the returned DOM
     * `Node` is imported into the current `Document`. If this flag is not enabled the
     * `Node` will belong (its ownerDocument) to a fresh `HTMLDocument`, created by
     * DOMPurify.
     *
     * This defaults to `true` starting DOMPurify 2.2.0. Note that setting it to `false`
     * might cause XSS from attacks hidden in closed shadowroots in case the browser
     * supports Declarative Shadow: DOM https://web.dev/declarative-shadow-dom/
     */
    var RETURN_DOM_IMPORT = true;

    /* Try to return a Trusted Type object instead of a string, return a string in
     * case Trusted Types are not supported  */
    var RETURN_TRUSTED_TYPE = false;

    /* Output should be free from DOM clobbering attacks? */
    var SANITIZE_DOM = true;

    /* Keep element content when removing element? */
    var KEEP_CONTENT = true;

    /* If a `Node` is passed to sanitize(), then performs sanitization in-place instead
     * of importing it into a new Document and returning a sanitized copy */
    var IN_PLACE = false;

    /* Allow usage of profiles like html, svg and mathMl */
    var USE_PROFILES = {};

    /* Tags to ignore content of when KEEP_CONTENT is true */
    var FORBID_CONTENTS = null;
    var DEFAULT_FORBID_CONTENTS = addToSet({}, ['annotation-xml', 'audio', 'colgroup', 'desc', 'foreignobject', 'head', 'iframe', 'math', 'mi', 'mn', 'mo', 'ms', 'mtext', 'noembed', 'noframes', 'noscript', 'plaintext', 'script', 'style', 'svg', 'template', 'thead', 'title', 'video', 'xmp']);

    /* Tags that are safe for data: URIs */
    var DATA_URI_TAGS = null;
    var DEFAULT_DATA_URI_TAGS = addToSet({}, ['audio', 'video', 'img', 'source', 'image', 'track']);

    /* Attributes safe for values like "javascript:" */
    var URI_SAFE_ATTRIBUTES = null;
    var DEFAULT_URI_SAFE_ATTRIBUTES = addToSet({}, ['alt', 'class', 'for', 'id', 'label', 'name', 'pattern', 'placeholder', 'role', 'summary', 'title', 'value', 'style', 'xmlns']);

    var MATHML_NAMESPACE = 'http://www.w3.org/1998/Math/MathML';
    var SVG_NAMESPACE = 'http://www.w3.org/2000/svg';
    var HTML_NAMESPACE = 'http://www.w3.org/1999/xhtml';
    /* Document namespace */
    var NAMESPACE = HTML_NAMESPACE;
    var IS_EMPTY_INPUT = false;

    /* Keep a reference to config to pass to hooks */
    var CONFIG = null;

    /* Ideally, do not touch anything below this line */
    /* ______________________________________________ */

    var formElement = document.createElement('form');

    /**
     * _parseConfig
     *
     * @param  {Object} cfg optional config literal
     */
    // eslint-disable-next-line complexity
    var _parseConfig = function _parseConfig(cfg) {
      if (CONFIG && CONFIG === cfg) {
        return;
      }

      /* Shield configuration object from tampering */
      if (!cfg || (typeof cfg === 'undefined' ? 'undefined' : _typeof(cfg)) !== 'object') {
        cfg = {};
      }

      /* Shield configuration object from prototype pollution */
      cfg = clone(cfg);

      /* Set configuration parameters */
      ALLOWED_TAGS = 'ALLOWED_TAGS' in cfg ? addToSet({}, cfg.ALLOWED_TAGS) : DEFAULT_ALLOWED_TAGS;
      ALLOWED_ATTR = 'ALLOWED_ATTR' in cfg ? addToSet({}, cfg.ALLOWED_ATTR) : DEFAULT_ALLOWED_ATTR;
      URI_SAFE_ATTRIBUTES = 'ADD_URI_SAFE_ATTR' in cfg ? addToSet(clone(DEFAULT_URI_SAFE_ATTRIBUTES), cfg.ADD_URI_SAFE_ATTR) : DEFAULT_URI_SAFE_ATTRIBUTES;
      DATA_URI_TAGS = 'ADD_DATA_URI_TAGS' in cfg ? addToSet(clone(DEFAULT_DATA_URI_TAGS), cfg.ADD_DATA_URI_TAGS) : DEFAULT_DATA_URI_TAGS;
      FORBID_CONTENTS = 'FORBID_CONTENTS' in cfg ? addToSet({}, cfg.FORBID_CONTENTS) : DEFAULT_FORBID_CONTENTS;
      FORBID_TAGS = 'FORBID_TAGS' in cfg ? addToSet({}, cfg.FORBID_TAGS) : {};
      FORBID_ATTR = 'FORBID_ATTR' in cfg ? addToSet({}, cfg.FORBID_ATTR) : {};
      USE_PROFILES = 'USE_PROFILES' in cfg ? cfg.USE_PROFILES : false;
      ALLOW_ARIA_ATTR = cfg.ALLOW_ARIA_ATTR !== false; // Default true
      ALLOW_DATA_ATTR = cfg.ALLOW_DATA_ATTR !== false; // Default true
      ALLOW_UNKNOWN_PROTOCOLS = cfg.ALLOW_UNKNOWN_PROTOCOLS || false; // Default false
      SAFE_FOR_TEMPLATES = cfg.SAFE_FOR_TEMPLATES || false; // Default false
      WHOLE_DOCUMENT = cfg.WHOLE_DOCUMENT || false; // Default false
      RETURN_DOM = cfg.RETURN_DOM || false; // Default false
      RETURN_DOM_FRAGMENT = cfg.RETURN_DOM_FRAGMENT || false; // Default false
      RETURN_DOM_IMPORT = cfg.RETURN_DOM_IMPORT !== false; // Default true
      RETURN_TRUSTED_TYPE = cfg.RETURN_TRUSTED_TYPE || false; // Default false
      FORCE_BODY = cfg.FORCE_BODY || false; // Default false
      SANITIZE_DOM = cfg.SANITIZE_DOM !== false; // Default true
      KEEP_CONTENT = cfg.KEEP_CONTENT !== false; // Default true
      IN_PLACE = cfg.IN_PLACE || false; // Default false
      IS_ALLOWED_URI$$1 = cfg.ALLOWED_URI_REGEXP || IS_ALLOWED_URI$$1;
      NAMESPACE = cfg.NAMESPACE || HTML_NAMESPACE;
      if (SAFE_FOR_TEMPLATES) {
        ALLOW_DATA_ATTR = false;
      }

      if (RETURN_DOM_FRAGMENT) {
        RETURN_DOM = true;
      }

      /* Parse profile info */
      if (USE_PROFILES) {
        ALLOWED_TAGS = addToSet({}, [].concat(_toConsumableArray$1(text)));
        ALLOWED_ATTR = [];
        if (USE_PROFILES.html === true) {
          addToSet(ALLOWED_TAGS, html);
          addToSet(ALLOWED_ATTR, html$1);
        }

        if (USE_PROFILES.svg === true) {
          addToSet(ALLOWED_TAGS, svg);
          addToSet(ALLOWED_ATTR, svg$1);
          addToSet(ALLOWED_ATTR, xml);
        }

        if (USE_PROFILES.svgFilters === true) {
          addToSet(ALLOWED_TAGS, svgFilters);
          addToSet(ALLOWED_ATTR, svg$1);
          addToSet(ALLOWED_ATTR, xml);
        }

        if (USE_PROFILES.mathMl === true) {
          addToSet(ALLOWED_TAGS, mathMl);
          addToSet(ALLOWED_ATTR, mathMl$1);
          addToSet(ALLOWED_ATTR, xml);
        }
      }

      /* Merge configuration parameters */
      if (cfg.ADD_TAGS) {
        if (ALLOWED_TAGS === DEFAULT_ALLOWED_TAGS) {
          ALLOWED_TAGS = clone(ALLOWED_TAGS);
        }

        addToSet(ALLOWED_TAGS, cfg.ADD_TAGS);
      }

      if (cfg.ADD_ATTR) {
        if (ALLOWED_ATTR === DEFAULT_ALLOWED_ATTR) {
          ALLOWED_ATTR = clone(ALLOWED_ATTR);
        }

        addToSet(ALLOWED_ATTR, cfg.ADD_ATTR);
      }

      if (cfg.ADD_URI_SAFE_ATTR) {
        addToSet(URI_SAFE_ATTRIBUTES, cfg.ADD_URI_SAFE_ATTR);
      }

      if (cfg.FORBID_CONTENTS) {
        if (FORBID_CONTENTS === DEFAULT_FORBID_CONTENTS) {
          FORBID_CONTENTS = clone(FORBID_CONTENTS);
        }

        addToSet(FORBID_CONTENTS, cfg.FORBID_CONTENTS);
      }

      /* Add #text in case KEEP_CONTENT is set to true */
      if (KEEP_CONTENT) {
        ALLOWED_TAGS['#text'] = true;
      }

      /* Add html, head and body to ALLOWED_TAGS in case WHOLE_DOCUMENT is true */
      if (WHOLE_DOCUMENT) {
        addToSet(ALLOWED_TAGS, ['html', 'head', 'body']);
      }

      /* Add tbody to ALLOWED_TAGS in case tables are permitted, see #286, #365 */
      if (ALLOWED_TAGS.table) {
        addToSet(ALLOWED_TAGS, ['tbody']);
        delete FORBID_TAGS.tbody;
      }

      // Prevent further manipulation of configuration.
      // Not available in IE8, Safari 5, etc.
      if (freeze) {
        freeze(cfg);
      }

      CONFIG = cfg;
    };

    var MATHML_TEXT_INTEGRATION_POINTS = addToSet({}, ['mi', 'mo', 'mn', 'ms', 'mtext']);

    var HTML_INTEGRATION_POINTS = addToSet({}, ['foreignobject', 'desc', 'title', 'annotation-xml']);

    /* Keep track of all possible SVG and MathML tags
     * so that we can perform the namespace checks
     * correctly. */
    var ALL_SVG_TAGS = addToSet({}, svg);
    addToSet(ALL_SVG_TAGS, svgFilters);
    addToSet(ALL_SVG_TAGS, svgDisallowed);

    var ALL_MATHML_TAGS = addToSet({}, mathMl);
    addToSet(ALL_MATHML_TAGS, mathMlDisallowed);

    /**
     *
     *
     * @param  {Element} element a DOM element whose namespace is being checked
     * @returns {boolean} Return false if the element has a
     *  namespace that a spec-compliant parser would never
     *  return. Return true otherwise.
     */
    var _checkValidNamespace = function _checkValidNamespace(element) {
      var parent = getParentNode(element);

      // In JSDOM, if we're inside shadow DOM, then parentNode
      // can be null. We just simulate parent in this case.
      if (!parent || !parent.tagName) {
        parent = {
          namespaceURI: HTML_NAMESPACE,
          tagName: 'template'
        };
      }

      var tagName = stringToLowerCase(element.tagName);
      var parentTagName = stringToLowerCase(parent.tagName);

      if (element.namespaceURI === SVG_NAMESPACE) {
        // The only way to switch from HTML namespace to SVG
        // is via <svg>. If it happens via any other tag, then
        // it should be killed.
        if (parent.namespaceURI === HTML_NAMESPACE) {
          return tagName === 'svg';
        }

        // The only way to switch from MathML to SVG is via
        // svg if parent is either <annotation-xml> or MathML
        // text integration points.
        if (parent.namespaceURI === MATHML_NAMESPACE) {
          return tagName === 'svg' && (parentTagName === 'annotation-xml' || MATHML_TEXT_INTEGRATION_POINTS[parentTagName]);
        }

        // We only allow elements that are defined in SVG
        // spec. All others are disallowed in SVG namespace.
        return Boolean(ALL_SVG_TAGS[tagName]);
      }

      if (element.namespaceURI === MATHML_NAMESPACE) {
        // The only way to switch from HTML namespace to MathML
        // is via <math>. If it happens via any other tag, then
        // it should be killed.
        if (parent.namespaceURI === HTML_NAMESPACE) {
          return tagName === 'math';
        }

        // The only way to switch from SVG to MathML is via
        // <math> and HTML integration points
        if (parent.namespaceURI === SVG_NAMESPACE) {
          return tagName === 'math' && HTML_INTEGRATION_POINTS[parentTagName];
        }

        // We only allow elements that are defined in MathML
        // spec. All others are disallowed in MathML namespace.
        return Boolean(ALL_MATHML_TAGS[tagName]);
      }

      if (element.namespaceURI === HTML_NAMESPACE) {
        // The only way to switch from SVG to HTML is via
        // HTML integration points, and from MathML to HTML
        // is via MathML text integration points
        if (parent.namespaceURI === SVG_NAMESPACE && !HTML_INTEGRATION_POINTS[parentTagName]) {
          return false;
        }

        if (parent.namespaceURI === MATHML_NAMESPACE && !MATHML_TEXT_INTEGRATION_POINTS[parentTagName]) {
          return false;
        }

        // Certain elements are allowed in both SVG and HTML
        // namespace. We need to specify them explicitly
        // so that they don't get erronously deleted from
        // HTML namespace.
        var commonSvgAndHTMLElements = addToSet({}, ['title', 'style', 'font', 'a', 'script']);

        // We disallow tags that are specific for MathML
        // or SVG and should never appear in HTML namespace
        return !ALL_MATHML_TAGS[tagName] && (commonSvgAndHTMLElements[tagName] || !ALL_SVG_TAGS[tagName]);
      }

      // The code should never reach this place (this means
      // that the element somehow got namespace that is not
      // HTML, SVG or MathML). Return false just in case.
      return false;
    };

    /**
     * _forceRemove
     *
     * @param  {Node} node a DOM node
     */
    var _forceRemove = function _forceRemove(node) {
      arrayPush(DOMPurify.removed, { element: node });
      try {
        // eslint-disable-next-line unicorn/prefer-dom-node-remove
        node.parentNode.removeChild(node);
      } catch (_) {
        try {
          node.outerHTML = emptyHTML;
        } catch (_) {
          node.remove();
        }
      }
    };

    /**
     * _removeAttribute
     *
     * @param  {String} name an Attribute name
     * @param  {Node} node a DOM node
     */
    var _removeAttribute = function _removeAttribute(name, node) {
      try {
        arrayPush(DOMPurify.removed, {
          attribute: node.getAttributeNode(name),
          from: node
        });
      } catch (_) {
        arrayPush(DOMPurify.removed, {
          attribute: null,
          from: node
        });
      }

      node.removeAttribute(name);

      // We void attribute values for unremovable "is"" attributes
      if (name === 'is' && !ALLOWED_ATTR[name]) {
        if (RETURN_DOM || RETURN_DOM_FRAGMENT) {
          try {
            _forceRemove(node);
          } catch (_) {}
        } else {
          try {
            node.setAttribute(name, '');
          } catch (_) {}
        }
      }
    };

    /**
     * _initDocument
     *
     * @param  {String} dirty a string of dirty markup
     * @return {Document} a DOM, filled with the dirty markup
     */
    var _initDocument = function _initDocument(dirty) {
      /* Create a HTML document */
      var doc = void 0;
      var leadingWhitespace = void 0;

      if (FORCE_BODY) {
        dirty = '<remove></remove>' + dirty;
      } else {
        /* If FORCE_BODY isn't used, leading whitespace needs to be preserved manually */
        var matches = stringMatch(dirty, /^[\r\n\t ]+/);
        leadingWhitespace = matches && matches[0];
      }

      var dirtyPayload = trustedTypesPolicy ? trustedTypesPolicy.createHTML(dirty) : dirty;
      /*
       * Use the DOMParser API by default, fallback later if needs be
       * DOMParser not work for svg when has multiple root element.
       */
      if (NAMESPACE === HTML_NAMESPACE) {
        try {
          doc = new DOMParser().parseFromString(dirtyPayload, 'text/html');
        } catch (_) {}
      }

      /* Use createHTMLDocument in case DOMParser is not available */
      if (!doc || !doc.documentElement) {
        doc = implementation.createDocument(NAMESPACE, 'template', null);
        try {
          doc.documentElement.innerHTML = IS_EMPTY_INPUT ? '' : dirtyPayload;
        } catch (_) {
          // Syntax error if dirtyPayload is invalid xml
        }
      }

      var body = doc.body || doc.documentElement;

      if (dirty && leadingWhitespace) {
        body.insertBefore(document.createTextNode(leadingWhitespace), body.childNodes[0] || null);
      }

      /* Work on whole document or just its body */
      if (NAMESPACE === HTML_NAMESPACE) {
        return getElementsByTagName.call(doc, WHOLE_DOCUMENT ? 'html' : 'body')[0];
      }

      return WHOLE_DOCUMENT ? doc.documentElement : body;
    };

    /**
     * _createIterator
     *
     * @param  {Document} root document/fragment to create iterator for
     * @return {Iterator} iterator instance
     */
    var _createIterator = function _createIterator(root) {
      return createNodeIterator.call(root.ownerDocument || root, root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_COMMENT | NodeFilter.SHOW_TEXT, null, false);
    };

    /**
     * _isClobbered
     *
     * @param  {Node} elm element to check for clobbering attacks
     * @return {Boolean} true if clobbered, false if safe
     */
    var _isClobbered = function _isClobbered(elm) {
      if (elm instanceof Text || elm instanceof Comment) {
        return false;
      }

      if (typeof elm.nodeName !== 'string' || typeof elm.textContent !== 'string' || typeof elm.removeChild !== 'function' || !(elm.attributes instanceof NamedNodeMap) || typeof elm.removeAttribute !== 'function' || typeof elm.setAttribute !== 'function' || typeof elm.namespaceURI !== 'string' || typeof elm.insertBefore !== 'function') {
        return true;
      }

      return false;
    };

    /**
     * _isNode
     *
     * @param  {Node} obj object to check whether it's a DOM node
     * @return {Boolean} true is object is a DOM node
     */
    var _isNode = function _isNode(object) {
      return (typeof Node === 'undefined' ? 'undefined' : _typeof(Node)) === 'object' ? object instanceof Node : object && (typeof object === 'undefined' ? 'undefined' : _typeof(object)) === 'object' && typeof object.nodeType === 'number' && typeof object.nodeName === 'string';
    };

    /**
     * _executeHook
     * Execute user configurable hooks
     *
     * @param  {String} entryPoint  Name of the hook's entry point
     * @param  {Node} currentNode node to work on with the hook
     * @param  {Object} data additional hook parameters
     */
    var _executeHook = function _executeHook(entryPoint, currentNode, data) {
      if (!hooks[entryPoint]) {
        return;
      }

      arrayForEach(hooks[entryPoint], function (hook) {
        hook.call(DOMPurify, currentNode, data, CONFIG);
      });
    };

    /**
     * _sanitizeElements
     *
     * @protect nodeName
     * @protect textContent
     * @protect removeChild
     *
     * @param   {Node} currentNode to check for permission to exist
     * @return  {Boolean} true if node was killed, false if left alive
     */
    var _sanitizeElements = function _sanitizeElements(currentNode) {
      var content = void 0;

      /* Execute a hook if present */
      _executeHook('beforeSanitizeElements', currentNode, null);

      /* Check if element is clobbered or can clobber */
      if (_isClobbered(currentNode)) {
        _forceRemove(currentNode);
        return true;
      }

      /* Check if tagname contains Unicode */
      if (stringMatch(currentNode.nodeName, /[\u0080-\uFFFF]/)) {
        _forceRemove(currentNode);
        return true;
      }

      /* Now let's check the element's type and name */
      var tagName = stringToLowerCase(currentNode.nodeName);

      /* Execute a hook if present */
      _executeHook('uponSanitizeElement', currentNode, {
        tagName: tagName,
        allowedTags: ALLOWED_TAGS
      });

      /* Detect mXSS attempts abusing namespace confusion */
      if (!_isNode(currentNode.firstElementChild) && (!_isNode(currentNode.content) || !_isNode(currentNode.content.firstElementChild)) && regExpTest(/<[/\w]/g, currentNode.innerHTML) && regExpTest(/<[/\w]/g, currentNode.textContent)) {
        _forceRemove(currentNode);
        return true;
      }

      /* Mitigate a problem with templates inside select */
      if (tagName === 'select' && regExpTest(/<template/i, currentNode.innerHTML)) {
        _forceRemove(currentNode);
        return true;
      }

      /* Remove element if anything forbids its presence */
      if (!ALLOWED_TAGS[tagName] || FORBID_TAGS[tagName]) {
        /* Keep content except for bad-listed elements */
        if (KEEP_CONTENT && !FORBID_CONTENTS[tagName]) {
          var parentNode = getParentNode(currentNode) || currentNode.parentNode;
          var childNodes = getChildNodes(currentNode) || currentNode.childNodes;

          if (childNodes && parentNode) {
            var childCount = childNodes.length;

            for (var i = childCount - 1; i >= 0; --i) {
              parentNode.insertBefore(cloneNode(childNodes[i], true), getNextSibling(currentNode));
            }
          }
        }

        _forceRemove(currentNode);
        return true;
      }

      /* Check whether element has a valid namespace */
      if (currentNode instanceof Element && !_checkValidNamespace(currentNode)) {
        _forceRemove(currentNode);
        return true;
      }

      if ((tagName === 'noscript' || tagName === 'noembed') && regExpTest(/<\/no(script|embed)/i, currentNode.innerHTML)) {
        _forceRemove(currentNode);
        return true;
      }

      /* Sanitize element content to be template-safe */
      if (SAFE_FOR_TEMPLATES && currentNode.nodeType === 3) {
        /* Get the element's text content */
        content = currentNode.textContent;
        content = stringReplace(content, MUSTACHE_EXPR$$1, ' ');
        content = stringReplace(content, ERB_EXPR$$1, ' ');
        if (currentNode.textContent !== content) {
          arrayPush(DOMPurify.removed, { element: currentNode.cloneNode() });
          currentNode.textContent = content;
        }
      }

      /* Execute a hook if present */
      _executeHook('afterSanitizeElements', currentNode, null);

      return false;
    };

    /**
     * _isValidAttribute
     *
     * @param  {string} lcTag Lowercase tag name of containing element.
     * @param  {string} lcName Lowercase attribute name.
     * @param  {string} value Attribute value.
     * @return {Boolean} Returns true if `value` is valid, otherwise false.
     */
    // eslint-disable-next-line complexity
    var _isValidAttribute = function _isValidAttribute(lcTag, lcName, value) {
      /* Make sure attribute cannot clobber */
      if (SANITIZE_DOM && (lcName === 'id' || lcName === 'name') && (value in document || value in formElement)) {
        return false;
      }

      /* Allow valid data-* attributes: At least one character after "-"
          (https://html.spec.whatwg.org/multipage/dom.html#embedding-custom-non-visible-data-with-the-data-*-attributes)
          XML-compatible (https://html.spec.whatwg.org/multipage/infrastructure.html#xml-compatible and http://www.w3.org/TR/xml/#d0e804)
          We don't need to check the value; it's always URI safe. */
      if (ALLOW_DATA_ATTR && !FORBID_ATTR[lcName] && regExpTest(DATA_ATTR$$1, lcName)) ; else if (ALLOW_ARIA_ATTR && regExpTest(ARIA_ATTR$$1, lcName)) ; else if (!ALLOWED_ATTR[lcName] || FORBID_ATTR[lcName]) {
        return false;

        /* Check value is safe. First, is attr inert? If so, is safe */
      } else if (URI_SAFE_ATTRIBUTES[lcName]) ; else if (regExpTest(IS_ALLOWED_URI$$1, stringReplace(value, ATTR_WHITESPACE$$1, ''))) ; else if ((lcName === 'src' || lcName === 'xlink:href' || lcName === 'href') && lcTag !== 'script' && stringIndexOf(value, 'data:') === 0 && DATA_URI_TAGS[lcTag]) ; else if (ALLOW_UNKNOWN_PROTOCOLS && !regExpTest(IS_SCRIPT_OR_DATA$$1, stringReplace(value, ATTR_WHITESPACE$$1, ''))) ; else if (!value) ; else {
        return false;
      }

      return true;
    };

    /**
     * _sanitizeAttributes
     *
     * @protect attributes
     * @protect nodeName
     * @protect removeAttribute
     * @protect setAttribute
     *
     * @param  {Node} currentNode to sanitize
     */
    var _sanitizeAttributes = function _sanitizeAttributes(currentNode) {
      var attr = void 0;
      var value = void 0;
      var lcName = void 0;
      var l = void 0;
      /* Execute a hook if present */
      _executeHook('beforeSanitizeAttributes', currentNode, null);

      var attributes = currentNode.attributes;

      /* Check if we have attributes; if not we might have a text node */

      if (!attributes) {
        return;
      }

      var hookEvent = {
        attrName: '',
        attrValue: '',
        keepAttr: true,
        allowedAttributes: ALLOWED_ATTR
      };
      l = attributes.length;

      /* Go backwards over all attributes; safely remove bad ones */
      while (l--) {
        attr = attributes[l];
        var _attr = attr,
            name = _attr.name,
            namespaceURI = _attr.namespaceURI;

        value = stringTrim(attr.value);
        lcName = stringToLowerCase(name);

        /* Execute a hook if present */
        hookEvent.attrName = lcName;
        hookEvent.attrValue = value;
        hookEvent.keepAttr = true;
        hookEvent.forceKeepAttr = undefined; // Allows developers to see this is a property they can set
        _executeHook('uponSanitizeAttribute', currentNode, hookEvent);
        value = hookEvent.attrValue;
        /* Did the hooks approve of the attribute? */
        if (hookEvent.forceKeepAttr) {
          continue;
        }

        /* Remove attribute */
        _removeAttribute(name, currentNode);

        /* Did the hooks approve of the attribute? */
        if (!hookEvent.keepAttr) {
          continue;
        }

        /* Work around a security issue in jQuery 3.0 */
        if (regExpTest(/\/>/i, value)) {
          _removeAttribute(name, currentNode);
          continue;
        }

        /* Sanitize attribute content to be template-safe */
        if (SAFE_FOR_TEMPLATES) {
          value = stringReplace(value, MUSTACHE_EXPR$$1, ' ');
          value = stringReplace(value, ERB_EXPR$$1, ' ');
        }

        /* Is `value` valid for this attribute? */
        var lcTag = currentNode.nodeName.toLowerCase();
        if (!_isValidAttribute(lcTag, lcName, value)) {
          continue;
        }

        /* Handle invalid data-* attribute set by try-catching it */
        try {
          if (namespaceURI) {
            currentNode.setAttributeNS(namespaceURI, name, value);
          } else {
            /* Fallback to setAttribute() for browser-unrecognized namespaces e.g. "x-schema". */
            currentNode.setAttribute(name, value);
          }

          arrayPop(DOMPurify.removed);
        } catch (_) {}
      }

      /* Execute a hook if present */
      _executeHook('afterSanitizeAttributes', currentNode, null);
    };

    /**
     * _sanitizeShadowDOM
     *
     * @param  {DocumentFragment} fragment to iterate over recursively
     */
    var _sanitizeShadowDOM = function _sanitizeShadowDOM(fragment) {
      var shadowNode = void 0;
      var shadowIterator = _createIterator(fragment);

      /* Execute a hook if present */
      _executeHook('beforeSanitizeShadowDOM', fragment, null);

      while (shadowNode = shadowIterator.nextNode()) {
        /* Execute a hook if present */
        _executeHook('uponSanitizeShadowNode', shadowNode, null);

        /* Sanitize tags and elements */
        if (_sanitizeElements(shadowNode)) {
          continue;
        }

        /* Deep shadow DOM detected */
        if (shadowNode.content instanceof DocumentFragment) {
          _sanitizeShadowDOM(shadowNode.content);
        }

        /* Check attributes, sanitize if necessary */
        _sanitizeAttributes(shadowNode);
      }

      /* Execute a hook if present */
      _executeHook('afterSanitizeShadowDOM', fragment, null);
    };

    /**
     * Sanitize
     * Public method providing core sanitation functionality
     *
     * @param {String|Node} dirty string or DOM node
     * @param {Object} configuration object
     */
    // eslint-disable-next-line complexity
    DOMPurify.sanitize = function (dirty, cfg) {
      var body = void 0;
      var importedNode = void 0;
      var currentNode = void 0;
      var oldNode = void 0;
      var returnNode = void 0;
      /* Make sure we have a string to sanitize.
        DO NOT return early, as this will return the wrong type if
        the user has requested a DOM object rather than a string */
      IS_EMPTY_INPUT = !dirty;
      if (IS_EMPTY_INPUT) {
        dirty = '<!-->';
      }

      /* Stringify, in case dirty is an object */
      if (typeof dirty !== 'string' && !_isNode(dirty)) {
        // eslint-disable-next-line no-negated-condition
        if (typeof dirty.toString !== 'function') {
          throw typeErrorCreate('toString is not a function');
        } else {
          dirty = dirty.toString();
          if (typeof dirty !== 'string') {
            throw typeErrorCreate('dirty is not a string, aborting');
          }
        }
      }

      /* Check we can run. Otherwise fall back or ignore */
      if (!DOMPurify.isSupported) {
        if (_typeof(window.toStaticHTML) === 'object' || typeof window.toStaticHTML === 'function') {
          if (typeof dirty === 'string') {
            return window.toStaticHTML(dirty);
          }

          if (_isNode(dirty)) {
            return window.toStaticHTML(dirty.outerHTML);
          }
        }

        return dirty;
      }

      /* Assign config vars */
      if (!SET_CONFIG) {
        _parseConfig(cfg);
      }

      /* Clean up removed elements */
      DOMPurify.removed = [];

      /* Check if dirty is correctly typed for IN_PLACE */
      if (typeof dirty === 'string') {
        IN_PLACE = false;
      }

      if (IN_PLACE) ; else if (dirty instanceof Node) {
        /* If dirty is a DOM element, append to an empty document to avoid
           elements being stripped by the parser */
        body = _initDocument('<!---->');
        importedNode = body.ownerDocument.importNode(dirty, true);
        if (importedNode.nodeType === 1 && importedNode.nodeName === 'BODY') {
          /* Node is already a body, use as is */
          body = importedNode;
        } else if (importedNode.nodeName === 'HTML') {
          body = importedNode;
        } else {
          // eslint-disable-next-line unicorn/prefer-dom-node-append
          body.appendChild(importedNode);
        }
      } else {
        /* Exit directly if we have nothing to do */
        if (!RETURN_DOM && !SAFE_FOR_TEMPLATES && !WHOLE_DOCUMENT &&
        // eslint-disable-next-line unicorn/prefer-includes
        dirty.indexOf('<') === -1) {
          return trustedTypesPolicy && RETURN_TRUSTED_TYPE ? trustedTypesPolicy.createHTML(dirty) : dirty;
        }

        /* Initialize the document to work on */
        body = _initDocument(dirty);

        /* Check we have a DOM node from the data */
        if (!body) {
          return RETURN_DOM ? null : emptyHTML;
        }
      }

      /* Remove first element node (ours) if FORCE_BODY is set */
      if (body && FORCE_BODY) {
        _forceRemove(body.firstChild);
      }

      /* Get node iterator */
      var nodeIterator = _createIterator(IN_PLACE ? dirty : body);

      /* Now start iterating over the created document */
      while (currentNode = nodeIterator.nextNode()) {
        /* Fix IE's strange behavior with manipulated textNodes #89 */
        if (currentNode.nodeType === 3 && currentNode === oldNode) {
          continue;
        }

        /* Sanitize tags and elements */
        if (_sanitizeElements(currentNode)) {
          continue;
        }

        /* Shadow DOM detected, sanitize it */
        if (currentNode.content instanceof DocumentFragment) {
          _sanitizeShadowDOM(currentNode.content);
        }

        /* Check attributes, sanitize if necessary */
        _sanitizeAttributes(currentNode);

        oldNode = currentNode;
      }

      oldNode = null;

      /* If we sanitized `dirty` in-place, return it. */
      if (IN_PLACE) {
        return dirty;
      }

      /* Return sanitized string or DOM */
      if (RETURN_DOM) {
        if (RETURN_DOM_FRAGMENT) {
          returnNode = createDocumentFragment.call(body.ownerDocument);

          while (body.firstChild) {
            // eslint-disable-next-line unicorn/prefer-dom-node-append
            returnNode.appendChild(body.firstChild);
          }
        } else {
          returnNode = body;
        }

        if (RETURN_DOM_IMPORT) {
          /*
            AdoptNode() is not used because internal state is not reset
            (e.g. the past names map of a HTMLFormElement), this is safe
            in theory but we would rather not risk another attack vector.
            The state that is cloned by importNode() is explicitly defined
            by the specs.
          */
          returnNode = importNode.call(originalDocument, returnNode, true);
        }

        return returnNode;
      }

      var serializedHTML = WHOLE_DOCUMENT ? body.outerHTML : body.innerHTML;

      /* Sanitize final string template-safe */
      if (SAFE_FOR_TEMPLATES) {
        serializedHTML = stringReplace(serializedHTML, MUSTACHE_EXPR$$1, ' ');
        serializedHTML = stringReplace(serializedHTML, ERB_EXPR$$1, ' ');
      }

      return trustedTypesPolicy && RETURN_TRUSTED_TYPE ? trustedTypesPolicy.createHTML(serializedHTML) : serializedHTML;
    };

    /**
     * Public method to set the configuration once
     * setConfig
     *
     * @param {Object} cfg configuration object
     */
    DOMPurify.setConfig = function (cfg) {
      _parseConfig(cfg);
      SET_CONFIG = true;
    };

    /**
     * Public method to remove the configuration
     * clearConfig
     *
     */
    DOMPurify.clearConfig = function () {
      CONFIG = null;
      SET_CONFIG = false;
    };

    /**
     * Public method to check if an attribute value is valid.
     * Uses last set config, if any. Otherwise, uses config defaults.
     * isValidAttribute
     *
     * @param  {string} tag Tag name of containing element.
     * @param  {string} attr Attribute name.
     * @param  {string} value Attribute value.
     * @return {Boolean} Returns true if `value` is valid. Otherwise, returns false.
     */
    DOMPurify.isValidAttribute = function (tag, attr, value) {
      /* Initialize shared config vars if necessary. */
      if (!CONFIG) {
        _parseConfig({});
      }

      var lcTag = stringToLowerCase(tag);
      var lcName = stringToLowerCase(attr);
      return _isValidAttribute(lcTag, lcName, value);
    };

    /**
     * AddHook
     * Public method to add DOMPurify hooks
     *
     * @param {String} entryPoint entry point for the hook to add
     * @param {Function} hookFunction function to execute
     */
    DOMPurify.addHook = function (entryPoint, hookFunction) {
      if (typeof hookFunction !== 'function') {
        return;
      }

      hooks[entryPoint] = hooks[entryPoint] || [];
      arrayPush(hooks[entryPoint], hookFunction);
    };

    /**
     * RemoveHook
     * Public method to remove a DOMPurify hook at a given entryPoint
     * (pops it from the stack of hooks if more are present)
     *
     * @param {String} entryPoint entry point for the hook to remove
     */
    DOMPurify.removeHook = function (entryPoint) {
      if (hooks[entryPoint]) {
        arrayPop(hooks[entryPoint]);
      }
    };

    /**
     * RemoveHooks
     * Public method to remove all DOMPurify hooks at a given entryPoint
     *
     * @param  {String} entryPoint entry point for the hooks to remove
     */
    DOMPurify.removeHooks = function (entryPoint) {
      if (hooks[entryPoint]) {
        hooks[entryPoint] = [];
      }
    };

    /**
     * RemoveAllHooks
     * Public method to remove all DOMPurify hooks
     *
     */
    DOMPurify.removeAllHooks = function () {
      hooks = {};
    };

    return DOMPurify;
  }

  var purify = createDOMPurify();

  var barEl;
  var timeId;

  /**
   * Init progress component
   */
  function init() {
    var div = create('div');

    div.classList.add('progress');
    appendTo(body, div);
    barEl = div;
  }

  /**
   * Render progress bar
   */
  function progressbar(ref) {
    var loaded = ref.loaded;
    var total = ref.total;
    var step = ref.step;

    var num;

    !barEl && init();

    if (step) {
      num = parseInt(barEl.style.width || 0, 10) + step;
      num = num > 80 ? 80 : num;
    } else {
      num = Math.floor((loaded / total) * 100);
    }

    barEl.style.opacity = 1;
    barEl.style.width = num >= 95 ? '100%' : num + '%';

    if (num >= 95) {
      clearTimeout(timeId);
      // eslint-disable-next-line no-unused-vars
      timeId = setTimeout(function (_) {
        barEl.style.opacity = 0;
        barEl.style.width = '0%';
      }, 200);
    }
  }

  /* eslint-disable no-unused-vars */

  var cache = {};

  /**
   * Ajax GET implmentation
   * @param {string} url Resource URL
   * @param {boolean} [hasBar=false] Has progress bar
   * @param {String[]} headers Array of headers
   * @return {Promise} Promise response
   */
  function get(url, hasBar, headers) {
    if ( hasBar === void 0 ) hasBar = false;
    if ( headers === void 0 ) headers = {};

    var xhr = new XMLHttpRequest();
    var on = function() {
      xhr.addEventListener.apply(xhr, arguments);
    };

    var cached = cache[url];

    if (cached) {
      return { then: function (cb) { return cb(cached.content, cached.opt); }, abort: noop };
    }

    xhr.open('GET', url);
    for (var i in headers) {
      if (hasOwn.call(headers, i)) {
        xhr.setRequestHeader(i, headers[i]);
      }
    }

    xhr.send();

    return {
      then: function(success, error) {
        if ( error === void 0 ) error = noop;

        if (hasBar) {
          var id = setInterval(
            function (_) { return progressbar({
                step: Math.floor(Math.random() * 5 + 1),
              }); },
            500
          );

          on('progress', progressbar);
          on('loadend', function (evt) {
            progressbar(evt);
            clearInterval(id);
          });
        }

        on('error', error);
        on('load', function (ref) {
          var target = ref.target;

          if (target.status >= 400) {
            error(target);
          } else {
            var result = (cache[url] = {
              content: target.response,
              opt: {
                updatedAt: xhr.getResponseHeader('last-modified'),
              },
            });

            success(result.content, result.opt);
          }
        });
      },
      abort: function (_) { return xhr.readyState !== 4 && xhr.abort(); },
    };
  }

  function replaceVar(block, color) {
    block.innerHTML = block.innerHTML.replace(
      /var\(\s*--theme-color.*?\)/g,
      color
    );
  }

  function cssVars(color) {
    // Variable support
    if (window.CSS && window.CSS.supports && window.CSS.supports('(--v:red)')) {
      return;
    }

    var styleBlocks = findAll('style:not(.inserted),link');
    [].forEach.call(styleBlocks, function (block) {
      if (block.nodeName === 'STYLE') {
        replaceVar(block, color);
      } else if (block.nodeName === 'LINK') {
        var href = block.getAttribute('href');

        if (!/\.css$/.test(href)) {
          return;
        }

        get(href).then(function (res) {
          var style = create('style', res);

          head.appendChild(style);
          replaceVar(style, color);
        });
      }
    });
  }

  /* eslint-disable no-unused-vars */

  var title = $.title;
  /**
   * Toggle button
   * @param {Element} el Button to be toggled
   * @void
   */
  function btn(el) {
    var toggle = function (_) { return body.classList.toggle('close'); };

    el = getNode(el);
    if (el === null || el === undefined) {
      return;
    }

    on(el, 'click', function (e) {
      e.stopPropagation();
      toggle();
    });

    isMobile &&
      on(
        body,
        'click',
        function (_) { return body.classList.contains('close') && toggle(); }
      );
  }

  function collapse(el) {
    el = getNode(el);
    if (el === null || el === undefined) {
      return;
    }

    on(el, 'click', function (ref) {
      var target = ref.target;

      if (
        target.nodeName === 'A' &&
        target.nextSibling &&
        target.nextSibling.classList &&
        target.nextSibling.classList.contains('app-sub-sidebar')
      ) {
        toggleClass(target.parentNode, 'collapse');
      }
    });
  }

  function sticky() {
    var cover = getNode('section.cover');
    if (!cover) {
      return;
    }

    var coverHeight = cover.getBoundingClientRect().height;

    if (window.pageYOffset >= coverHeight || cover.classList.contains('hidden')) {
      toggleClass(body, 'add', 'sticky');
    } else {
      toggleClass(body, 'remove', 'sticky');
    }
  }

  /**
   * Get and active link
   * @param  {Object} router Router
   * @param  {String|Element} el Target element
   * @param  {Boolean} isParent Active parent
   * @param  {Boolean} autoTitle Automatically set title
   * @return {Element} Active element
   */
  function getAndActive(router, el, isParent, autoTitle) {
    el = getNode(el);
    var links = [];
    if (el !== null && el !== undefined) {
      links = findAll(el, 'a');
    }

    var hash = decodeURI(router.toURL(router.getCurrentPath()));
    var target;

    links
      .sort(function (a, b) { return b.href.length - a.href.length; })
      .forEach(function (a) {
        var href = decodeURI(a.getAttribute('href'));
        var node = isParent ? a.parentNode : a;

        a.title = a.title || a.innerText;

        if (hash.indexOf(href) === 0 && !target) {
          target = a;
          toggleClass(node, 'add', 'active');
        } else {
          toggleClass(node, 'remove', 'active');
        }
      });

    if (autoTitle) {
      $.title = target
        ? target.title || ((target.innerText) + " - " + title)
        : title;
    }

    return target;
  }

  var _createClass = function () { function defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) { descriptor.writable = true; } Object.defineProperty(target, descriptor.key, descriptor); } } return function (Constructor, protoProps, staticProps) { if (protoProps) { defineProperties(Constructor.prototype, protoProps); } if (staticProps) { defineProperties(Constructor, staticProps); } return Constructor; }; }();

  function _classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

  var SingleTweener = function () {
    function SingleTweener() {
      var opts = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : {};

      _classCallCheck(this, SingleTweener);

      this.start = opts.start;
      this.end = opts.end;
      this.decimal = opts.decimal;
    }

    _createClass(SingleTweener, [{
      key: "getIntermediateValue",
      value: function getIntermediateValue(tick) {
        if (this.decimal) {
          return tick;
        } else {
          return Math.round(tick);
        }
      }
    }, {
      key: "getFinalValue",
      value: function getFinalValue() {
        return this.end;
      }
    }]);

    return SingleTweener;
  }();

  var _createClass$1 = function () { function defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) { descriptor.writable = true; } Object.defineProperty(target, descriptor.key, descriptor); } } return function (Constructor, protoProps, staticProps) { if (protoProps) { defineProperties(Constructor.prototype, protoProps); } if (staticProps) { defineProperties(Constructor, staticProps); } return Constructor; }; }();

  function _classCallCheck$1(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

  var Tweezer = function () {
    function Tweezer() {
      var opts = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : {};

      _classCallCheck$1(this, Tweezer);

      this.duration = opts.duration || 1000;
      this.ease = opts.easing || this._defaultEase;
      this.tweener = opts.tweener || new SingleTweener(opts);
      this.start = this.tweener.start;
      this.end = this.tweener.end;

      this.frame = null;
      this.next = null;
      this.isRunning = false;
      this.events = {};
      this.direction = this.start < this.end ? 'up' : 'down';
    }

    _createClass$1(Tweezer, [{
      key: 'begin',
      value: function begin() {
        if (!this.isRunning && this.next !== this.end) {
          this.frame = window.requestAnimationFrame(this._tick.bind(this));
        }
        return this;
      }
    }, {
      key: 'stop',
      value: function stop() {
        window.cancelAnimationFrame(this.frame);
        this.isRunning = false;
        this.frame = null;
        this.timeStart = null;
        this.next = null;
        return this;
      }
    }, {
      key: 'on',
      value: function on(name, handler) {
        this.events[name] = this.events[name] || [];
        this.events[name].push(handler);
        return this;
      }
    }, {
      key: '_emit',
      value: function _emit(name, val) {
        var _this = this;

        var e = this.events[name];
        e && e.forEach(function (handler) {
          return handler.call(_this, val);
        });
      }
    }, {
      key: '_tick',
      value: function _tick(currentTime) {
        this.isRunning = true;

        var lastTick = this.next || this.start;

        if (!this.timeStart) { this.timeStart = currentTime; }
        this.timeElapsed = currentTime - this.timeStart;
        this.next = this.ease(this.timeElapsed, this.start, this.end - this.start, this.duration);

        if (this._shouldTick(lastTick)) {
          this._emit('tick', this.tweener.getIntermediateValue(this.next));
          this.frame = window.requestAnimationFrame(this._tick.bind(this));
        } else {
          this._emit('tick', this.tweener.getFinalValue());
          this._emit('done', null);
        }
      }
    }, {
      key: '_shouldTick',
      value: function _shouldTick(lastTick) {
        return {
          up: this.next < this.end && lastTick <= this.next,
          down: this.next > this.end && lastTick >= this.next
        }[this.direction];
      }
    }, {
      key: '_defaultEase',
      value: function _defaultEase(t, b, c, d) {
        if ((t /= d / 2) < 1) { return c / 2 * t * t + b; }
        return -c / 2 * (--t * (t - 2) - 1) + b;
      }
    }]);

    return Tweezer;
  }();

  var currentScript = document.currentScript;

  /** @param {import('./Docsify').Docsify} vm */
  function config(vm) {
    var config = merge(
      {
        el: '#app',
        repo: '',
        maxLevel: 6,
        subMaxLevel: 0,
        loadSidebar: null,
        loadNavbar: null,
        homepage: 'README.md',
        coverpage: '',
        basePath: '',
        auto2top: false,
        name: '',
        themeColor: '',
        nameLink: window.location.pathname,
        autoHeader: false,
        executeScript: null,
        noEmoji: false,
        ga: '',
        ext: '.md',
        mergeNavbar: false,
        formatUpdated: '',
        // This config for the links inside markdown
        externalLinkTarget: '_blank',
        // This config for the corner
        cornerExternalLinkTarget: '_blank',
        externalLinkRel: 'noopener',
        routerMode: 'hash',
        noCompileLinks: [],
        crossOriginLinks: [],
        relativePath: false,
        topMargin: 0,
      },
      typeof window.$docsify === 'function'
        ? window.$docsify(vm)
        : window.$docsify
    );

    var script =
      currentScript ||
      [].slice
        .call(document.getElementsByTagName('script'))
        .filter(function (n) { return /docsify\./.test(n.src); })[0];

    if (script) {
      for (var prop in config) {
        if (hasOwn.call(config, prop)) {
          var val = script.getAttribute('data-' + hyphenate(prop));

          if (isPrimitive(val)) {
            config[prop] = val === '' ? true : val;
          }
        }
      }
    }

    if (config.loadSidebar === true) {
      config.loadSidebar = '_sidebar' + config.ext;
    }

    if (config.loadNavbar === true) {
      config.loadNavbar = '_navbar' + config.ext;
    }

    if (config.coverpage === true) {
      config.coverpage = '_coverpage' + config.ext;
    }

    if (config.repo === true) {
      config.repo = '';
    }

    if (config.name === true) {
      config.name = '';
    }

    window.$docsify = config;

    return config;
  }

  var nav = {};
  var hoverOver = false;
  var scroller = null;
  var enableScrollEvent = true;
  var coverHeight = 0;

  function scrollTo(el, offset) {
    if ( offset === void 0 ) offset = 0;

    if (scroller) {
      scroller.stop();
    }

    enableScrollEvent = false;
    scroller = new Tweezer({
      start: window.pageYOffset,
      end:
        Math.round(el.getBoundingClientRect().top) + window.pageYOffset - offset,
      duration: 500,
    })
      .on('tick', function (v) { return window.scrollTo(0, v); })
      .on('done', function () {
        enableScrollEvent = true;
        scroller = null;
      })
      .begin();
  }

  function highlight(path) {
    if (!enableScrollEvent) {
      return;
    }

    var sidebar = getNode('.sidebar');
    var anchors = findAll('.anchor');
    var wrap = find(sidebar, '.sidebar-nav');
    var active = find(sidebar, 'li.active');
    var doc = document.documentElement;
    var top = ((doc && doc.scrollTop) || document.body.scrollTop) - coverHeight;
    var last;

    for (var i = 0, len = anchors.length; i < len; i += 1) {
      var node = anchors[i];

      if (node.offsetTop > top) {
        if (!last) {
          last = node;
        }

        break;
      } else {
        last = node;
      }
    }

    if (!last) {
      return;
    }

    var li = nav[getNavKey(path, last.getAttribute('data-id'))];

    if (!li || li === active) {
      return;
    }

    active && active.classList.remove('active');
    li.classList.add('active');
    active = li;

    // Scroll into view
    // https://github.com/vuejs/vuejs.org/blob/master/themes/vue/source/js/common.js#L282-L297
    if (!hoverOver && body.classList.contains('sticky')) {
      var height = sidebar.clientHeight;
      var curOffset = 0;
      var cur = active.offsetTop + active.clientHeight + 40;
      var isInView =
        active.offsetTop >= wrap.scrollTop && cur <= wrap.scrollTop + height;
      var notThan = cur - curOffset < height;

      sidebar.scrollTop = isInView
        ? wrap.scrollTop
        : notThan
        ? curOffset
        : cur - height;
    }
  }

  function getNavKey(path, id) {
    return ((decodeURIComponent(path)) + "?id=" + (decodeURIComponent(id)));
  }

  function scrollActiveSidebar(router) {
    var cover = find('.cover.show');
    coverHeight = cover ? cover.offsetHeight : 0;

    var sidebar = getNode('.sidebar');
    var lis = [];
    if (sidebar !== null && sidebar !== undefined) {
      lis = findAll(sidebar, 'li');
    }

    for (var i = 0, len = lis.length; i < len; i += 1) {
      var li = lis[i];
      var a = li.querySelector('a');
      if (!a) {
        continue;
      }

      var href = a.getAttribute('href');

      if (href !== '/') {
        var ref = router.parse(href);
        var id = ref.query.id;
        var path$1 = ref.path;
        if (id) {
          href = getNavKey(path$1, id);
        }
      }

      if (href) {
        nav[decodeURIComponent(href)] = li;
      }
    }

    if (isMobile) {
      return;
    }

    var path = removeParams(router.getCurrentPath());
    off('scroll', function () { return highlight(path); });
    on('scroll', function () { return highlight(path); });
    on(sidebar, 'mouseover', function () {
      hoverOver = true;
    });
    on(sidebar, 'mouseleave', function () {
      hoverOver = false;
    });
  }

  function scrollIntoView(path, id) {
    if (!id) {
      return;
    }
    var topMargin = config().topMargin;
    var section = find('#' + id);
    section && scrollTo(section, topMargin);

    var li = nav[getNavKey(path, id)];
    var sidebar = getNode('.sidebar');
    var active = find(sidebar, 'li.active');
    active && active.classList.remove('active');
    li && li.classList.add('active');
  }

  var scrollEl = $.scrollingElement || $.documentElement;

  function scroll2Top(offset) {
    if ( offset === void 0 ) offset = 0;

    scrollEl.scrollTop = offset === true ? 0 : Number(offset);
  }

  var commonjsGlobal = typeof globalThis !== 'undefined' ? globalThis : typeof window !== 'undefined' ? window : typeof global !== 'undefined' ? global : typeof self !== 'undefined' ? self : {};

  function createCommonjsModule(fn, module) {
  	return module = { exports: {} }, fn(module, module.exports), module.exports;
  }

  var defaults = createCommonjsModule(function (module) {
  function getDefaults() {
    return {
      baseUrl: null,
      breaks: false,
      gfm: true,
      headerIds: true,
      headerPrefix: '',
      highlight: null,
      langPrefix: 'language-',
      mangle: true,
      pedantic: false,
      renderer: null,
      sanitize: false,
      sanitizer: null,
      silent: false,
      smartLists: false,
      smartypants: false,
      tokenizer: null,
      walkTokens: null,
      xhtml: false
    };
  }

  function changeDefaults(newDefaults) {
    module.exports.defaults = newDefaults;
  }

  module.exports = {
    defaults: getDefaults(),
    getDefaults: getDefaults,
    changeDefaults: changeDefaults
  };
  });
  var defaults_1 = defaults.defaults;
  var defaults_2 = defaults.getDefaults;
  var defaults_3 = defaults.changeDefaults;

  /**
   * Helpers
   */
  var escapeTest = /[&<>"']/;
  var escapeReplace = /[&<>"']/g;
  var escapeTestNoEncode = /[<>"']|&(?!#?\w+;)/;
  var escapeReplaceNoEncode = /[<>"']|&(?!#?\w+;)/g;
  var escapeReplacements = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  };
  var getEscapeReplacement = function (ch) { return escapeReplacements[ch]; };
  function escape(html, encode) {
    if (encode) {
      if (escapeTest.test(html)) {
        return html.replace(escapeReplace, getEscapeReplacement);
      }
    } else {
      if (escapeTestNoEncode.test(html)) {
        return html.replace(escapeReplaceNoEncode, getEscapeReplacement);
      }
    }

    return html;
  }

  var unescapeTest = /&(#(?:\d+)|(?:#x[0-9A-Fa-f]+)|(?:\w+));?/ig;

  function unescape(html) {
    // explicitly match decimal, hex, and named HTML entities
    return html.replace(unescapeTest, function (_, n) {
      n = n.toLowerCase();
      if (n === 'colon') { return ':'; }
      if (n.charAt(0) === '#') {
        return n.charAt(1) === 'x'
          ? String.fromCharCode(parseInt(n.substring(2), 16))
          : String.fromCharCode(+n.substring(1));
      }
      return '';
    });
  }

  var caret = /(^|[^\[])\^/g;
  function edit(regex, opt) {
    regex = regex.source || regex;
    opt = opt || '';
    var obj = {
      replace: function (name, val) {
        val = val.source || val;
        val = val.replace(caret, '$1');
        regex = regex.replace(name, val);
        return obj;
      },
      getRegex: function () {
        return new RegExp(regex, opt);
      }
    };
    return obj;
  }

  var nonWordAndColonTest = /[^\w:]/g;
  var originIndependentUrl = /^$|^[a-z][a-z0-9+.-]*:|^[?#]/i;
  function cleanUrl(sanitize, base, href) {
    if (sanitize) {
      var prot;
      try {
        prot = decodeURIComponent(unescape(href))
          .replace(nonWordAndColonTest, '')
          .toLowerCase();
      } catch (e) {
        return null;
      }
      if (prot.indexOf('javascript:') === 0 || prot.indexOf('vbscript:') === 0 || prot.indexOf('data:') === 0) {
        return null;
      }
    }
    if (base && !originIndependentUrl.test(href)) {
      href = resolveUrl(base, href);
    }
    try {
      href = encodeURI(href).replace(/%25/g, '%');
    } catch (e) {
      return null;
    }
    return href;
  }

  var baseUrls = {};
  var justDomain = /^[^:]+:\/*[^/]*$/;
  var protocol = /^([^:]+:)[\s\S]*$/;
  var domain = /^([^:]+:\/*[^/]*)[\s\S]*$/;

  function resolveUrl(base, href) {
    if (!baseUrls[' ' + base]) {
      // we can ignore everything in base after the last slash of its path component,
      // but we might need to add _that_
      // https://tools.ietf.org/html/rfc3986#section-3
      if (justDomain.test(base)) {
        baseUrls[' ' + base] = base + '/';
      } else {
        baseUrls[' ' + base] = rtrim(base, '/', true);
      }
    }
    base = baseUrls[' ' + base];
    var relativeBase = base.indexOf(':') === -1;

    if (href.substring(0, 2) === '//') {
      if (relativeBase) {
        return href;
      }
      return base.replace(protocol, '$1') + href;
    } else if (href.charAt(0) === '/') {
      if (relativeBase) {
        return href;
      }
      return base.replace(domain, '$1') + href;
    } else {
      return base + href;
    }
  }

  var noopTest = { exec: function noopTest() {} };

  function merge$1(obj) {
    var arguments$1 = arguments;

    var i = 1,
      target,
      key;

    for (; i < arguments.length; i++) {
      target = arguments$1[i];
      for (key in target) {
        if (Object.prototype.hasOwnProperty.call(target, key)) {
          obj[key] = target[key];
        }
      }
    }

    return obj;
  }

  function splitCells(tableRow, count) {
    // ensure that every cell-delimiting pipe has a space
    // before it to distinguish it from an escaped pipe
    var row = tableRow.replace(/\|/g, function (match, offset, str) {
        var escaped = false,
          curr = offset;
        while (--curr >= 0 && str[curr] === '\\') { escaped = !escaped; }
        if (escaped) {
          // odd number of slashes means | is escaped
          // so we leave it alone
          return '|';
        } else {
          // add space before unescaped |
          return ' |';
        }
      }),
      cells = row.split(/ \|/);
    var i = 0;

    if (cells.length > count) {
      cells.splice(count);
    } else {
      while (cells.length < count) { cells.push(''); }
    }

    for (; i < cells.length; i++) {
      // leading or trailing whitespace is ignored per the gfm spec
      cells[i] = cells[i].trim().replace(/\\\|/g, '|');
    }
    return cells;
  }

  // Remove trailing 'c's. Equivalent to str.replace(/c*$/, '').
  // /c*$/ is vulnerable to REDOS.
  // invert: Remove suffix of non-c chars instead. Default falsey.
  function rtrim(str, c, invert) {
    var l = str.length;
    if (l === 0) {
      return '';
    }

    // Length of suffix matching the invert condition.
    var suffLen = 0;

    // Step left until we fail to match the invert condition.
    while (suffLen < l) {
      var currChar = str.charAt(l - suffLen - 1);
      if (currChar === c && !invert) {
        suffLen++;
      } else if (currChar !== c && invert) {
        suffLen++;
      } else {
        break;
      }
    }

    return str.substr(0, l - suffLen);
  }

  function findClosingBracket(str, b) {
    if (str.indexOf(b[1]) === -1) {
      return -1;
    }
    var l = str.length;
    var level = 0,
      i = 0;
    for (; i < l; i++) {
      if (str[i] === '\\') {
        i++;
      } else if (str[i] === b[0]) {
        level++;
      } else if (str[i] === b[1]) {
        level--;
        if (level < 0) {
          return i;
        }
      }
    }
    return -1;
  }

  function checkSanitizeDeprecation(opt) {
    if (opt && opt.sanitize && !opt.silent) {
      console.warn('marked(): sanitize and sanitizer parameters are deprecated since version 0.7.0, should not be used and will be removed in the future. Read more here: https://marked.js.org/#/USING_ADVANCED.md#options');
    }
  }

  // copied from https://stackoverflow.com/a/5450113/806777
  function repeatString(pattern, count) {
    if (count < 1) {
      return '';
    }
    var result = '';
    while (count > 1) {
      if (count & 1) {
        result += pattern;
      }
      count >>= 1;
      pattern += pattern;
    }
    return result + pattern;
  }

  var helpers = {
    escape: escape,
    unescape: unescape,
    edit: edit,
    cleanUrl: cleanUrl,
    resolveUrl: resolveUrl,
    noopTest: noopTest,
    merge: merge$1,
    splitCells: splitCells,
    rtrim: rtrim,
    findClosingBracket: findClosingBracket,
    checkSanitizeDeprecation: checkSanitizeDeprecation,
    repeatString: repeatString
  };

  var defaults$1 = defaults.defaults;

  var rtrim$1 = helpers.rtrim;
  var splitCells$1 = helpers.splitCells;
  var escape$1 = helpers.escape;
  var findClosingBracket$1 = helpers.findClosingBracket;

  function outputLink(cap, link, raw) {
    var href = link.href;
    var title = link.title ? escape$1(link.title) : null;
    var text = cap[1].replace(/\\([\[\]])/g, '$1');

    if (cap[0].charAt(0) !== '!') {
      return {
        type: 'link',
        raw: raw,
        href: href,
        title: title,
        text: text
      };
    } else {
      return {
        type: 'image',
        raw: raw,
        href: href,
        title: title,
        text: escape$1(text)
      };
    }
  }

  function indentCodeCompensation(raw, text) {
    var matchIndentToCode = raw.match(/^(\s+)(?:```)/);

    if (matchIndentToCode === null) {
      return text;
    }

    var indentToCode = matchIndentToCode[1];

    return text
      .split('\n')
      .map(function (node) {
        var matchIndentInNode = node.match(/^\s+/);
        if (matchIndentInNode === null) {
          return node;
        }

        var indentInNode = matchIndentInNode[0];

        if (indentInNode.length >= indentToCode.length) {
          return node.slice(indentToCode.length);
        }

        return node;
      })
      .join('\n');
  }

  /**
   * Tokenizer
   */
  var Tokenizer = /*@__PURE__*/(function () {
    function Tokenizer(options) {
      this.options = options || defaults$1;
    }

    Tokenizer.prototype.space = function space (src) {
      var cap = this.rules.block.newline.exec(src);
      if (cap) {
        if (cap[0].length > 1) {
          return {
            type: 'space',
            raw: cap[0]
          };
        }
        return { raw: '\n' };
      }
    };

    Tokenizer.prototype.code = function code (src, tokens) {
      var cap = this.rules.block.code.exec(src);
      if (cap) {
        var lastToken = tokens[tokens.length - 1];
        // An indented code block cannot interrupt a paragraph.
        if (lastToken && lastToken.type === 'paragraph') {
          return {
            raw: cap[0],
            text: cap[0].trimRight()
          };
        }

        var text = cap[0].replace(/^ {1,4}/gm, '');
        return {
          type: 'code',
          raw: cap[0],
          codeBlockStyle: 'indented',
          text: !this.options.pedantic
            ? rtrim$1(text, '\n')
            : text
        };
      }
    };

    Tokenizer.prototype.fences = function fences (src) {
      var cap = this.rules.block.fences.exec(src);
      if (cap) {
        var raw = cap[0];
        var text = indentCodeCompensation(raw, cap[3] || '');

        return {
          type: 'code',
          raw: raw,
          lang: cap[2] ? cap[2].trim() : cap[2],
          text: text
        };
      }
    };

    Tokenizer.prototype.heading = function heading (src) {
      var cap = this.rules.block.heading.exec(src);
      if (cap) {
        var text = cap[2].trim();

        // remove trailing #s
        if (/#$/.test(text)) {
          var trimmed = rtrim$1(text, '#');
          if (this.options.pedantic) {
            text = trimmed.trim();
          } else if (!trimmed || / $/.test(trimmed)) {
            // CommonMark requires space before trailing #s
            text = trimmed.trim();
          }
        }

        return {
          type: 'heading',
          raw: cap[0],
          depth: cap[1].length,
          text: text
        };
      }
    };

    Tokenizer.prototype.nptable = function nptable (src) {
      var cap = this.rules.block.nptable.exec(src);
      if (cap) {
        var item = {
          type: 'table',
          header: splitCells$1(cap[1].replace(/^ *| *\| *$/g, '')),
          align: cap[2].replace(/^ *|\| *$/g, '').split(/ *\| */),
          cells: cap[3] ? cap[3].replace(/\n$/, '').split('\n') : [],
          raw: cap[0]
        };

        if (item.header.length === item.align.length) {
          var l = item.align.length;
          var i;
          for (i = 0; i < l; i++) {
            if (/^ *-+: *$/.test(item.align[i])) {
              item.align[i] = 'right';
            } else if (/^ *:-+: *$/.test(item.align[i])) {
              item.align[i] = 'center';
            } else if (/^ *:-+ *$/.test(item.align[i])) {
              item.align[i] = 'left';
            } else {
              item.align[i] = null;
            }
          }

          l = item.cells.length;
          for (i = 0; i < l; i++) {
            item.cells[i] = splitCells$1(item.cells[i], item.header.length);
          }

          return item;
        }
      }
    };

    Tokenizer.prototype.hr = function hr (src) {
      var cap = this.rules.block.hr.exec(src);
      if (cap) {
        return {
          type: 'hr',
          raw: cap[0]
        };
      }
    };

    Tokenizer.prototype.blockquote = function blockquote (src) {
      var cap = this.rules.block.blockquote.exec(src);
      if (cap) {
        var text = cap[0].replace(/^ *> ?/gm, '');

        return {
          type: 'blockquote',
          raw: cap[0],
          text: text
        };
      }
    };

    Tokenizer.prototype.list = function list (src) {
      var cap = this.rules.block.list.exec(src);
      if (cap) {
        var raw = cap[0];
        var bull = cap[2];
        var isordered = bull.length > 1;

        var list = {
          type: 'list',
          raw: raw,
          ordered: isordered,
          start: isordered ? +bull.slice(0, -1) : '',
          loose: false,
          items: []
        };

        // Get each top-level item.
        var itemMatch = cap[0].match(this.rules.block.item);

        var next = false,
          item,
          space,
          bcurr,
          bnext,
          addBack,
          loose,
          istask,
          ischecked;

        var l = itemMatch.length;
        bcurr = this.rules.block.listItemStart.exec(itemMatch[0]);
        for (var i = 0; i < l; i++) {
          item = itemMatch[i];
          raw = item;

          // Determine whether the next list item belongs here.
          // Backpedal if it does not belong in this list.
          if (i !== l - 1) {
            bnext = this.rules.block.listItemStart.exec(itemMatch[i + 1]);
            if (
              !this.options.pedantic
                ? bnext[1].length > bcurr[0].length || bnext[1].length > 3
                : bnext[1].length > bcurr[1].length
            ) {
              // nested list
              itemMatch.splice(i, 2, itemMatch[i] + '\n' + itemMatch[i + 1]);
              i--;
              l--;
              continue;
            } else {
              if (
                // different bullet style
                !this.options.pedantic || this.options.smartLists
                  ? bnext[2][bnext[2].length - 1] !== bull[bull.length - 1]
                  : isordered === (bnext[2].length === 1)
              ) {
                addBack = itemMatch.slice(i + 1).join('\n');
                list.raw = list.raw.substring(0, list.raw.length - addBack.length);
                i = l - 1;
              }
            }
            bcurr = bnext;
          }

          // Remove the list item's bullet
          // so it is seen as the next token.
          space = item.length;
          item = item.replace(/^ *([*+-]|\d+[.)]) ?/, '');

          // Outdent whatever the
          // list item contains. Hacky.
          if (~item.indexOf('\n ')) {
            space -= item.length;
            item = !this.options.pedantic
              ? item.replace(new RegExp('^ {1,' + space + '}', 'gm'), '')
              : item.replace(/^ {1,4}/gm, '');
          }

          // Determine whether item is loose or not.
          // Use: /(^|\n)(?! )[^\n]+\n\n(?!\s*$)/
          // for discount behavior.
          loose = next || /\n\n(?!\s*$)/.test(item);
          if (i !== l - 1) {
            next = item.charAt(item.length - 1) === '\n';
            if (!loose) { loose = next; }
          }

          if (loose) {
            list.loose = true;
          }

          // Check for task list items
          if (this.options.gfm) {
            istask = /^\[[ xX]\] /.test(item);
            ischecked = undefined;
            if (istask) {
              ischecked = item[1] !== ' ';
              item = item.replace(/^\[[ xX]\] +/, '');
            }
          }

          list.items.push({
            type: 'list_item',
            raw: raw,
            task: istask,
            checked: ischecked,
            loose: loose,
            text: item
          });
        }

        return list;
      }
    };

    Tokenizer.prototype.html = function html (src) {
      var cap = this.rules.block.html.exec(src);
      if (cap) {
        return {
          type: this.options.sanitize
            ? 'paragraph'
            : 'html',
          raw: cap[0],
          pre: !this.options.sanitizer
            && (cap[1] === 'pre' || cap[1] === 'script' || cap[1] === 'style'),
          text: this.options.sanitize ? (this.options.sanitizer ? this.options.sanitizer(cap[0]) : escape$1(cap[0])) : cap[0]
        };
      }
    };

    Tokenizer.prototype.def = function def (src) {
      var cap = this.rules.block.def.exec(src);
      if (cap) {
        if (cap[3]) { cap[3] = cap[3].substring(1, cap[3].length - 1); }
        var tag = cap[1].toLowerCase().replace(/\s+/g, ' ');
        return {
          tag: tag,
          raw: cap[0],
          href: cap[2],
          title: cap[3]
        };
      }
    };

    Tokenizer.prototype.table = function table (src) {
      var cap = this.rules.block.table.exec(src);
      if (cap) {
        var item = {
          type: 'table',
          header: splitCells$1(cap[1].replace(/^ *| *\| *$/g, '')),
          align: cap[2].replace(/^ *|\| *$/g, '').split(/ *\| */),
          cells: cap[3] ? cap[3].replace(/\n$/, '').split('\n') : []
        };

        if (item.header.length === item.align.length) {
          item.raw = cap[0];

          var l = item.align.length;
          var i;
          for (i = 0; i < l; i++) {
            if (/^ *-+: *$/.test(item.align[i])) {
              item.align[i] = 'right';
            } else if (/^ *:-+: *$/.test(item.align[i])) {
              item.align[i] = 'center';
            } else if (/^ *:-+ *$/.test(item.align[i])) {
              item.align[i] = 'left';
            } else {
              item.align[i] = null;
            }
          }

          l = item.cells.length;
          for (i = 0; i < l; i++) {
            item.cells[i] = splitCells$1(
              item.cells[i].replace(/^ *\| *| *\| *$/g, ''),
              item.header.length);
          }

          return item;
        }
      }
    };

    Tokenizer.prototype.lheading = function lheading (src) {
      var cap = this.rules.block.lheading.exec(src);
      if (cap) {
        return {
          type: 'heading',
          raw: cap[0],
          depth: cap[2].charAt(0) === '=' ? 1 : 2,
          text: cap[1]
        };
      }
    };

    Tokenizer.prototype.paragraph = function paragraph (src) {
      var cap = this.rules.block.paragraph.exec(src);
      if (cap) {
        return {
          type: 'paragraph',
          raw: cap[0],
          text: cap[1].charAt(cap[1].length - 1) === '\n'
            ? cap[1].slice(0, -1)
            : cap[1]
        };
      }
    };

    Tokenizer.prototype.text = function text (src, tokens) {
      var cap = this.rules.block.text.exec(src);
      if (cap) {
        var lastToken = tokens[tokens.length - 1];
        if (lastToken && lastToken.type === 'text') {
          return {
            raw: cap[0],
            text: cap[0]
          };
        }

        return {
          type: 'text',
          raw: cap[0],
          text: cap[0]
        };
      }
    };

    Tokenizer.prototype.escape = function escape$1$1 (src) {
      var cap = this.rules.inline.escape.exec(src);
      if (cap) {
        return {
          type: 'escape',
          raw: cap[0],
          text: escape$1(cap[1])
        };
      }
    };

    Tokenizer.prototype.tag = function tag (src, inLink, inRawBlock) {
      var cap = this.rules.inline.tag.exec(src);
      if (cap) {
        if (!inLink && /^<a /i.test(cap[0])) {
          inLink = true;
        } else if (inLink && /^<\/a>/i.test(cap[0])) {
          inLink = false;
        }
        if (!inRawBlock && /^<(pre|code|kbd|script)(\s|>)/i.test(cap[0])) {
          inRawBlock = true;
        } else if (inRawBlock && /^<\/(pre|code|kbd|script)(\s|>)/i.test(cap[0])) {
          inRawBlock = false;
        }

        return {
          type: this.options.sanitize
            ? 'text'
            : 'html',
          raw: cap[0],
          inLink: inLink,
          inRawBlock: inRawBlock,
          text: this.options.sanitize
            ? (this.options.sanitizer
              ? this.options.sanitizer(cap[0])
              : escape$1(cap[0]))
            : cap[0]
        };
      }
    };

    Tokenizer.prototype.link = function link (src) {
      var cap = this.rules.inline.link.exec(src);
      if (cap) {
        var trimmedUrl = cap[2].trim();
        if (!this.options.pedantic && /^</.test(trimmedUrl)) {
          // commonmark requires matching angle brackets
          if (!(/>$/.test(trimmedUrl))) {
            return;
          }

          // ending angle bracket cannot be escaped
          var rtrimSlash = rtrim$1(trimmedUrl.slice(0, -1), '\\');
          if ((trimmedUrl.length - rtrimSlash.length) % 2 === 0) {
            return;
          }
        } else {
          // find closing parenthesis
          var lastParenIndex = findClosingBracket$1(cap[2], '()');
          if (lastParenIndex > -1) {
            var start = cap[0].indexOf('!') === 0 ? 5 : 4;
            var linkLen = start + cap[1].length + lastParenIndex;
            cap[2] = cap[2].substring(0, lastParenIndex);
            cap[0] = cap[0].substring(0, linkLen).trim();
            cap[3] = '';
          }
        }
        var href = cap[2];
        var title = '';
        if (this.options.pedantic) {
          // split pedantic href and title
          var link = /^([^'"]*[^\s])\s+(['"])(.*)\2/.exec(href);

          if (link) {
            href = link[1];
            title = link[3];
          }
        } else {
          title = cap[3] ? cap[3].slice(1, -1) : '';
        }

        href = href.trim();
        if (/^</.test(href)) {
          if (this.options.pedantic && !(/>$/.test(trimmedUrl))) {
            // pedantic allows starting angle bracket without ending angle bracket
            href = href.slice(1);
          } else {
            href = href.slice(1, -1);
          }
        }
        return outputLink(cap, {
          href: href ? href.replace(this.rules.inline._escapes, '$1') : href,
          title: title ? title.replace(this.rules.inline._escapes, '$1') : title
        }, cap[0]);
      }
    };

    Tokenizer.prototype.reflink = function reflink (src, links) {
      var cap;
      if ((cap = this.rules.inline.reflink.exec(src))
          || (cap = this.rules.inline.nolink.exec(src))) {
        var link = (cap[2] || cap[1]).replace(/\s+/g, ' ');
        link = links[link.toLowerCase()];
        if (!link || !link.href) {
          var text = cap[0].charAt(0);
          return {
            type: 'text',
            raw: text,
            text: text
          };
        }
        return outputLink(cap, link, cap[0]);
      }
    };

    Tokenizer.prototype.strong = function strong (src, maskedSrc, prevChar) {
      if ( prevChar === void 0 ) prevChar = '';

      var match = this.rules.inline.strong.start.exec(src);

      if (match && (!match[1] || (match[1] && (prevChar === '' || this.rules.inline.punctuation.exec(prevChar))))) {
        maskedSrc = maskedSrc.slice(-1 * src.length);
        var endReg = match[0] === '**' ? this.rules.inline.strong.endAst : this.rules.inline.strong.endUnd;

        endReg.lastIndex = 0;

        var cap;
        while ((match = endReg.exec(maskedSrc)) != null) {
          cap = this.rules.inline.strong.middle.exec(maskedSrc.slice(0, match.index + 3));
          if (cap) {
            return {
              type: 'strong',
              raw: src.slice(0, cap[0].length),
              text: src.slice(2, cap[0].length - 2)
            };
          }
        }
      }
    };

    Tokenizer.prototype.em = function em (src, maskedSrc, prevChar) {
      if ( prevChar === void 0 ) prevChar = '';

      var match = this.rules.inline.em.start.exec(src);

      if (match && (!match[1] || (match[1] && (prevChar === '' || this.rules.inline.punctuation.exec(prevChar))))) {
        maskedSrc = maskedSrc.slice(-1 * src.length);
        var endReg = match[0] === '*' ? this.rules.inline.em.endAst : this.rules.inline.em.endUnd;

        endReg.lastIndex = 0;

        var cap;
        while ((match = endReg.exec(maskedSrc)) != null) {
          cap = this.rules.inline.em.middle.exec(maskedSrc.slice(0, match.index + 2));
          if (cap) {
            return {
              type: 'em',
              raw: src.slice(0, cap[0].length),
              text: src.slice(1, cap[0].length - 1)
            };
          }
        }
      }
    };

    Tokenizer.prototype.codespan = function codespan (src) {
      var cap = this.rules.inline.code.exec(src);
      if (cap) {
        var text = cap[2].replace(/\n/g, ' ');
        var hasNonSpaceChars = /[^ ]/.test(text);
        var hasSpaceCharsOnBothEnds = /^ /.test(text) && / $/.test(text);
        if (hasNonSpaceChars && hasSpaceCharsOnBothEnds) {
          text = text.substring(1, text.length - 1);
        }
        text = escape$1(text, true);
        return {
          type: 'codespan',
          raw: cap[0],
          text: text
        };
      }
    };

    Tokenizer.prototype.br = function br (src) {
      var cap = this.rules.inline.br.exec(src);
      if (cap) {
        return {
          type: 'br',
          raw: cap[0]
        };
      }
    };

    Tokenizer.prototype.del = function del (src) {
      var cap = this.rules.inline.del.exec(src);
      if (cap) {
        return {
          type: 'del',
          raw: cap[0],
          text: cap[2]
        };
      }
    };

    Tokenizer.prototype.autolink = function autolink (src, mangle) {
      var cap = this.rules.inline.autolink.exec(src);
      if (cap) {
        var text, href;
        if (cap[2] === '@') {
          text = escape$1(this.options.mangle ? mangle(cap[1]) : cap[1]);
          href = 'mailto:' + text;
        } else {
          text = escape$1(cap[1]);
          href = text;
        }

        return {
          type: 'link',
          raw: cap[0],
          text: text,
          href: href,
          tokens: [
            {
              type: 'text',
              raw: text,
              text: text
            }
          ]
        };
      }
    };

    Tokenizer.prototype.url = function url (src, mangle) {
      var cap;
      if (cap = this.rules.inline.url.exec(src)) {
        var text, href;
        if (cap[2] === '@') {
          text = escape$1(this.options.mangle ? mangle(cap[0]) : cap[0]);
          href = 'mailto:' + text;
        } else {
          // do extended autolink path validation
          var prevCapZero;
          do {
            prevCapZero = cap[0];
            cap[0] = this.rules.inline._backpedal.exec(cap[0])[0];
          } while (prevCapZero !== cap[0]);
          text = escape$1(cap[0]);
          if (cap[1] === 'www.') {
            href = 'http://' + text;
          } else {
            href = text;
          }
        }
        return {
          type: 'link',
          raw: cap[0],
          text: text,
          href: href,
          tokens: [
            {
              type: 'text',
              raw: text,
              text: text
            }
          ]
        };
      }
    };

    Tokenizer.prototype.inlineText = function inlineText (src, inRawBlock, smartypants) {
      var cap = this.rules.inline.text.exec(src);
      if (cap) {
        var text;
        if (inRawBlock) {
          text = this.options.sanitize ? (this.options.sanitizer ? this.options.sanitizer(cap[0]) : escape$1(cap[0])) : cap[0];
        } else {
          text = escape$1(this.options.smartypants ? smartypants(cap[0]) : cap[0]);
        }
        return {
          type: 'text',
          raw: cap[0],
          text: text
        };
      }
    };

    return Tokenizer;
  }());

  var noopTest$1 = helpers.noopTest;
  var edit$1 = helpers.edit;
  var merge$2 = helpers.merge;

  /**
   * Block-Level Grammar
   */
  var block = {
    newline: /^(?: *(?:\n|$))+/,
    code: /^( {4}[^\n]+(?:\n(?: *(?:\n|$))*)?)+/,
    fences: /^ {0,3}(`{3,}(?=[^`\n]*\n)|~{3,})([^\n]*)\n(?:|([\s\S]*?)\n)(?: {0,3}\1[~`]* *(?:\n+|$)|$)/,
    hr: /^ {0,3}((?:- *){3,}|(?:_ *){3,}|(?:\* *){3,})(?:\n+|$)/,
    heading: /^ {0,3}(#{1,6})(?=\s|$)(.*)(?:\n+|$)/,
    blockquote: /^( {0,3}> ?(paragraph|[^\n]*)(?:\n|$))+/,
    list: /^( {0,3})(bull) [\s\S]+?(?:hr|def|\n{2,}(?! )(?! {0,3}bull )\n*|\s*$)/,
    html: '^ {0,3}(?:' // optional indentation
      + '<(script|pre|style)[\\s>][\\s\\S]*?(?:</\\1>[^\\n]*\\n+|$)' // (1)
      + '|comment[^\\n]*(\\n+|$)' // (2)
      + '|<\\?[\\s\\S]*?(?:\\?>\\n*|$)' // (3)
      + '|<![A-Z][\\s\\S]*?(?:>\\n*|$)' // (4)
      + '|<!\\[CDATA\\[[\\s\\S]*?(?:\\]\\]>\\n*|$)' // (5)
      + '|</?(tag)(?: +|\\n|/?>)[\\s\\S]*?(?:\\n{2,}|$)' // (6)
      + '|<(?!script|pre|style)([a-z][\\w-]*)(?:attribute)*? */?>(?=[ \\t]*(?:\\n|$))[\\s\\S]*?(?:\\n{2,}|$)' // (7) open tag
      + '|</(?!script|pre|style)[a-z][\\w-]*\\s*>(?=[ \\t]*(?:\\n|$))[\\s\\S]*?(?:\\n{2,}|$)' // (7) closing tag
      + ')',
    def: /^ {0,3}\[(label)\]: *\n? *<?([^\s>]+)>?(?:(?: +\n? *| *\n *)(title))? *(?:\n+|$)/,
    nptable: noopTest$1,
    table: noopTest$1,
    lheading: /^([^\n]+)\n {0,3}(=+|-+) *(?:\n+|$)/,
    // regex template, placeholders will be replaced according to different paragraph
    // interruption rules of commonmark and the original markdown spec:
    _paragraph: /^([^\n]+(?:\n(?!hr|heading|lheading|blockquote|fences|list|html| +\n)[^\n]+)*)/,
    text: /^[^\n]+/
  };

  block._label = /(?!\s*\])(?:\\[\[\]]|[^\[\]])+/;
  block._title = /(?:"(?:\\"?|[^"\\])*"|'[^'\n]*(?:\n[^'\n]+)*\n?'|\([^()]*\))/;
  block.def = edit$1(block.def)
    .replace('label', block._label)
    .replace('title', block._title)
    .getRegex();

  block.bullet = /(?:[*+-]|\d{1,9}[.)])/;
  block.item = /^( *)(bull) ?[^\n]*(?:\n(?! *bull ?)[^\n]*)*/;
  block.item = edit$1(block.item, 'gm')
    .replace(/bull/g, block.bullet)
    .getRegex();

  block.listItemStart = edit$1(/^( *)(bull)/)
    .replace('bull', block.bullet)
    .getRegex();

  block.list = edit$1(block.list)
    .replace(/bull/g, block.bullet)
    .replace('hr', '\\n+(?=\\1?(?:(?:- *){3,}|(?:_ *){3,}|(?:\\* *){3,})(?:\\n+|$))')
    .replace('def', '\\n+(?=' + block.def.source + ')')
    .getRegex();

  block._tag = 'address|article|aside|base|basefont|blockquote|body|caption'
    + '|center|col|colgroup|dd|details|dialog|dir|div|dl|dt|fieldset|figcaption'
    + '|figure|footer|form|frame|frameset|h[1-6]|head|header|hr|html|iframe'
    + '|legend|li|link|main|menu|menuitem|meta|nav|noframes|ol|optgroup|option'
    + '|p|param|section|source|summary|table|tbody|td|tfoot|th|thead|title|tr'
    + '|track|ul';
  block._comment = /<!--(?!-?>)[\s\S]*?(?:-->|$)/;
  block.html = edit$1(block.html, 'i')
    .replace('comment', block._comment)
    .replace('tag', block._tag)
    .replace('attribute', / +[a-zA-Z:_][\w.:-]*(?: *= *"[^"\n]*"| *= *'[^'\n]*'| *= *[^\s"'=<>`]+)?/)
    .getRegex();

  block.paragraph = edit$1(block._paragraph)
    .replace('hr', block.hr)
    .replace('heading', ' {0,3}#{1,6} ')
    .replace('|lheading', '') // setex headings don't interrupt commonmark paragraphs
    .replace('blockquote', ' {0,3}>')
    .replace('fences', ' {0,3}(?:`{3,}(?=[^`\\n]*\\n)|~{3,})[^\\n]*\\n')
    .replace('list', ' {0,3}(?:[*+-]|1[.)]) ') // only lists starting from 1 can interrupt
    .replace('html', '</?(?:tag)(?: +|\\n|/?>)|<(?:script|pre|style|!--)')
    .replace('tag', block._tag) // pars can be interrupted by type (6) html blocks
    .getRegex();

  block.blockquote = edit$1(block.blockquote)
    .replace('paragraph', block.paragraph)
    .getRegex();

  /**
   * Normal Block Grammar
   */

  block.normal = merge$2({}, block);

  /**
   * GFM Block Grammar
   */

  block.gfm = merge$2({}, block.normal, {
    nptable: '^ *([^|\\n ].*\\|.*)\\n' // Header
      + ' {0,3}([-:]+ *\\|[-| :]*)' // Align
      + '(?:\\n((?:(?!\\n|hr|heading|blockquote|code|fences|list|html).*(?:\\n|$))*)\\n*|$)', // Cells
    table: '^ *\\|(.+)\\n' // Header
      + ' {0,3}\\|?( *[-:]+[-| :]*)' // Align
      + '(?:\\n *((?:(?!\\n|hr|heading|blockquote|code|fences|list|html).*(?:\\n|$))*)\\n*|$)' // Cells
  });

  block.gfm.nptable = edit$1(block.gfm.nptable)
    .replace('hr', block.hr)
    .replace('heading', ' {0,3}#{1,6} ')
    .replace('blockquote', ' {0,3}>')
    .replace('code', ' {4}[^\\n]')
    .replace('fences', ' {0,3}(?:`{3,}(?=[^`\\n]*\\n)|~{3,})[^\\n]*\\n')
    .replace('list', ' {0,3}(?:[*+-]|1[.)]) ') // only lists starting from 1 can interrupt
    .replace('html', '</?(?:tag)(?: +|\\n|/?>)|<(?:script|pre|style|!--)')
    .replace('tag', block._tag) // tables can be interrupted by type (6) html blocks
    .getRegex();

  block.gfm.table = edit$1(block.gfm.table)
    .replace('hr', block.hr)
    .replace('heading', ' {0,3}#{1,6} ')
    .replace('blockquote', ' {0,3}>')
    .replace('code', ' {4}[^\\n]')
    .replace('fences', ' {0,3}(?:`{3,}(?=[^`\\n]*\\n)|~{3,})[^\\n]*\\n')
    .replace('list', ' {0,3}(?:[*+-]|1[.)]) ') // only lists starting from 1 can interrupt
    .replace('html', '</?(?:tag)(?: +|\\n|/?>)|<(?:script|pre|style|!--)')
    .replace('tag', block._tag) // tables can be interrupted by type (6) html blocks
    .getRegex();

  /**
   * Pedantic grammar (original John Gruber's loose markdown specification)
   */

  block.pedantic = merge$2({}, block.normal, {
    html: edit$1(
      '^ *(?:comment *(?:\\n|\\s*$)'
      + '|<(tag)[\\s\\S]+?</\\1> *(?:\\n{2,}|\\s*$)' // closed tag
      + '|<tag(?:"[^"]*"|\'[^\']*\'|\\s[^\'"/>\\s]*)*?/?> *(?:\\n{2,}|\\s*$))')
      .replace('comment', block._comment)
      .replace(/tag/g, '(?!(?:'
        + 'a|em|strong|small|s|cite|q|dfn|abbr|data|time|code|var|samp|kbd|sub'
        + '|sup|i|b|u|mark|ruby|rt|rp|bdi|bdo|span|br|wbr|ins|del|img)'
        + '\\b)\\w+(?!:|[^\\w\\s@]*@)\\b')
      .getRegex(),
    def: /^ *\[([^\]]+)\]: *<?([^\s>]+)>?(?: +(["(][^\n]+[")]))? *(?:\n+|$)/,
    heading: /^(#{1,6})(.*)(?:\n+|$)/,
    fences: noopTest$1, // fences not supported
    paragraph: edit$1(block.normal._paragraph)
      .replace('hr', block.hr)
      .replace('heading', ' *#{1,6} *[^\n]')
      .replace('lheading', block.lheading)
      .replace('blockquote', ' {0,3}>')
      .replace('|fences', '')
      .replace('|list', '')
      .replace('|html', '')
      .getRegex()
  });

  /**
   * Inline-Level Grammar
   */
  var inline = {
    escape: /^\\([!"#$%&'()*+,\-./:;<=>?@\[\]\\^_`{|}~])/,
    autolink: /^<(scheme:[^\s\x00-\x1f<>]*|email)>/,
    url: noopTest$1,
    tag: '^comment'
      + '|^</[a-zA-Z][\\w:-]*\\s*>' // self-closing tag
      + '|^<[a-zA-Z][\\w-]*(?:attribute)*?\\s*/?>' // open tag
      + '|^<\\?[\\s\\S]*?\\?>' // processing instruction, e.g. <?php ?>
      + '|^<![a-zA-Z]+\\s[\\s\\S]*?>' // declaration, e.g. <!DOCTYPE html>
      + '|^<!\\[CDATA\\[[\\s\\S]*?\\]\\]>', // CDATA section
    link: /^!?\[(label)\]\(\s*(href)(?:\s+(title))?\s*\)/,
    reflink: /^!?\[(label)\]\[(?!\s*\])((?:\\[\[\]]?|[^\[\]\\])+)\]/,
    nolink: /^!?\[(?!\s*\])((?:\[[^\[\]]*\]|\\[\[\]]|[^\[\]])*)\](?:\[\])?/,
    reflinkSearch: 'reflink|nolink(?!\\()',
    strong: {
      start: /^(?:(\*\*(?=[*punctuation]))|\*\*)(?![\s])|__/, // (1) returns if starts w/ punctuation
      middle: /^\*\*(?:(?:(?!overlapSkip)(?:[^*]|\\\*)|overlapSkip)|\*(?:(?!overlapSkip)(?:[^*]|\\\*)|overlapSkip)*?\*)+?\*\*$|^__(?![\s])((?:(?:(?!overlapSkip)(?:[^_]|\\_)|overlapSkip)|_(?:(?!overlapSkip)(?:[^_]|\\_)|overlapSkip)*?_)+?)__$/,
      endAst: /[^punctuation\s]\*\*(?!\*)|[punctuation]\*\*(?!\*)(?:(?=[punctuation_\s]|$))/, // last char can't be punct, or final * must also be followed by punct (or endline)
      endUnd: /[^\s]__(?!_)(?:(?=[punctuation*\s])|$)/ // last char can't be a space, and final _ must preceed punct or \s (or endline)
    },
    em: {
      start: /^(?:(\*(?=[punctuation]))|\*)(?![*\s])|_/, // (1) returns if starts w/ punctuation
      middle: /^\*(?:(?:(?!overlapSkip)(?:[^*]|\\\*)|overlapSkip)|\*(?:(?!overlapSkip)(?:[^*]|\\\*)|overlapSkip)*?\*)+?\*$|^_(?![_\s])(?:(?:(?!overlapSkip)(?:[^_]|\\_)|overlapSkip)|_(?:(?!overlapSkip)(?:[^_]|\\_)|overlapSkip)*?_)+?_$/,
      endAst: /[^punctuation\s]\*(?!\*)|[punctuation]\*(?!\*)(?:(?=[punctuation_\s]|$))/, // last char can't be punct, or final * must also be followed by punct (or endline)
      endUnd: /[^\s]_(?!_)(?:(?=[punctuation*\s])|$)/ // last char can't be a space, and final _ must preceed punct or \s (or endline)
    },
    code: /^(`+)([^`]|[^`][\s\S]*?[^`])\1(?!`)/,
    br: /^( {2,}|\\)\n(?!\s*$)/,
    del: noopTest$1,
    text: /^(`+|[^`])(?:(?= {2,}\n)|[\s\S]*?(?:(?=[\\<!\[`*]|\b_|$)|[^ ](?= {2,}\n)))/,
    punctuation: /^([\s*punctuation])/
  };

  // list of punctuation marks from common mark spec
  // without * and _ to workaround cases with double emphasis
  inline._punctuation = '!"#$%&\'()+\\-.,/:;<=>?@\\[\\]`^{|}~';
  inline.punctuation = edit$1(inline.punctuation).replace(/punctuation/g, inline._punctuation).getRegex();

  // sequences em should skip over [title](link), `code`, <html>
  inline._blockSkip = '\\[[^\\]]*?\\]\\([^\\)]*?\\)|`[^`]*?`|<[^>]*?>';
  inline._overlapSkip = '__[^_]*?__|\\*\\*\\[^\\*\\]*?\\*\\*';

  inline._comment = edit$1(block._comment).replace('(?:-->|$)', '-->').getRegex();

  inline.em.start = edit$1(inline.em.start)
    .replace(/punctuation/g, inline._punctuation)
    .getRegex();

  inline.em.middle = edit$1(inline.em.middle)
    .replace(/punctuation/g, inline._punctuation)
    .replace(/overlapSkip/g, inline._overlapSkip)
    .getRegex();

  inline.em.endAst = edit$1(inline.em.endAst, 'g')
    .replace(/punctuation/g, inline._punctuation)
    .getRegex();

  inline.em.endUnd = edit$1(inline.em.endUnd, 'g')
    .replace(/punctuation/g, inline._punctuation)
    .getRegex();

  inline.strong.start = edit$1(inline.strong.start)
    .replace(/punctuation/g, inline._punctuation)
    .getRegex();

  inline.strong.middle = edit$1(inline.strong.middle)
    .replace(/punctuation/g, inline._punctuation)
    .replace(/overlapSkip/g, inline._overlapSkip)
    .getRegex();

  inline.strong.endAst = edit$1(inline.strong.endAst, 'g')
    .replace(/punctuation/g, inline._punctuation)
    .getRegex();

  inline.strong.endUnd = edit$1(inline.strong.endUnd, 'g')
    .replace(/punctuation/g, inline._punctuation)
    .getRegex();

  inline.blockSkip = edit$1(inline._blockSkip, 'g')
    .getRegex();

  inline.overlapSkip = edit$1(inline._overlapSkip, 'g')
    .getRegex();

  inline._escapes = /\\([!"#$%&'()*+,\-./:;<=>?@\[\]\\^_`{|}~])/g;

  inline._scheme = /[a-zA-Z][a-zA-Z0-9+.-]{1,31}/;
  inline._email = /[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+(@)[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+(?![-_])/;
  inline.autolink = edit$1(inline.autolink)
    .replace('scheme', inline._scheme)
    .replace('email', inline._email)
    .getRegex();

  inline._attribute = /\s+[a-zA-Z:_][\w.:-]*(?:\s*=\s*"[^"]*"|\s*=\s*'[^']*'|\s*=\s*[^\s"'=<>`]+)?/;

  inline.tag = edit$1(inline.tag)
    .replace('comment', inline._comment)
    .replace('attribute', inline._attribute)
    .getRegex();

  inline._label = /(?:\[(?:\\.|[^\[\]\\])*\]|\\.|`[^`]*`|[^\[\]\\`])*?/;
  inline._href = /<(?:\\.|[^\n<>\\])+>|[^\s\x00-\x1f]*/;
  inline._title = /"(?:\\"?|[^"\\])*"|'(?:\\'?|[^'\\])*'|\((?:\\\)?|[^)\\])*\)/;

  inline.link = edit$1(inline.link)
    .replace('label', inline._label)
    .replace('href', inline._href)
    .replace('title', inline._title)
    .getRegex();

  inline.reflink = edit$1(inline.reflink)
    .replace('label', inline._label)
    .getRegex();

  inline.reflinkSearch = edit$1(inline.reflinkSearch, 'g')
    .replace('reflink', inline.reflink)
    .replace('nolink', inline.nolink)
    .getRegex();

  /**
   * Normal Inline Grammar
   */

  inline.normal = merge$2({}, inline);

  /**
   * Pedantic Inline Grammar
   */

  inline.pedantic = merge$2({}, inline.normal, {
    strong: {
      start: /^__|\*\*/,
      middle: /^__(?=\S)([\s\S]*?\S)__(?!_)|^\*\*(?=\S)([\s\S]*?\S)\*\*(?!\*)/,
      endAst: /\*\*(?!\*)/g,
      endUnd: /__(?!_)/g
    },
    em: {
      start: /^_|\*/,
      middle: /^()\*(?=\S)([\s\S]*?\S)\*(?!\*)|^_(?=\S)([\s\S]*?\S)_(?!_)/,
      endAst: /\*(?!\*)/g,
      endUnd: /_(?!_)/g
    },
    link: edit$1(/^!?\[(label)\]\((.*?)\)/)
      .replace('label', inline._label)
      .getRegex(),
    reflink: edit$1(/^!?\[(label)\]\s*\[([^\]]*)\]/)
      .replace('label', inline._label)
      .getRegex()
  });

  /**
   * GFM Inline Grammar
   */

  inline.gfm = merge$2({}, inline.normal, {
    escape: edit$1(inline.escape).replace('])', '~|])').getRegex(),
    _extended_email: /[A-Za-z0-9._+-]+(@)[a-zA-Z0-9-_]+(?:\.[a-zA-Z0-9-_]*[a-zA-Z0-9])+(?![-_])/,
    url: /^((?:ftp|https?):\/\/|www\.)(?:[a-zA-Z0-9\-]+\.?)+[^\s<]*|^email/,
    _backpedal: /(?:[^?!.,:;*_~()&]+|\([^)]*\)|&(?![a-zA-Z0-9]+;$)|[?!.,:;*_~)]+(?!$))+/,
    del: /^(~~?)(?=[^\s~])([\s\S]*?[^\s~])\1(?=[^~]|$)/,
    text: /^([`~]+|[^`~])(?:(?= {2,}\n)|[\s\S]*?(?:(?=[\\<!\[`*~]|\b_|https?:\/\/|ftp:\/\/|www\.|$)|[^ ](?= {2,}\n)|[^a-zA-Z0-9.!#$%&'*+\/=?_`{\|}~-](?=[a-zA-Z0-9.!#$%&'*+\/=?_`{\|}~-]+@))|(?=[a-zA-Z0-9.!#$%&'*+\/=?_`{\|}~-]+@))/
  });

  inline.gfm.url = edit$1(inline.gfm.url, 'i')
    .replace('email', inline.gfm._extended_email)
    .getRegex();
  /**
   * GFM + Line Breaks Inline Grammar
   */

  inline.breaks = merge$2({}, inline.gfm, {
    br: edit$1(inline.br).replace('{2,}', '*').getRegex(),
    text: edit$1(inline.gfm.text)
      .replace('\\b_', '\\b_| {2,}\\n')
      .replace(/\{2,\}/g, '*')
      .getRegex()
  });

  var rules = {
    block: block,
    inline: inline
  };

  var defaults$2 = defaults.defaults;

  var block$1 = rules.block;
  var inline$1 = rules.inline;

  var repeatString$1 = helpers.repeatString;

  /**
   * smartypants text replacement
   */
  function smartypants(text) {
    return text
      // em-dashes
      .replace(/---/g, '\u2014')
      // en-dashes
      .replace(/--/g, '\u2013')
      // opening singles
      .replace(/(^|[-\u2014/(\[{"\s])'/g, '$1\u2018')
      // closing singles & apostrophes
      .replace(/'/g, '\u2019')
      // opening doubles
      .replace(/(^|[-\u2014/(\[{\u2018\s])"/g, '$1\u201c')
      // closing doubles
      .replace(/"/g, '\u201d')
      // ellipses
      .replace(/\.{3}/g, '\u2026');
  }

  /**
   * mangle email addresses
   */
  function mangle(text) {
    var out = '',
      i,
      ch;

    var l = text.length;
    for (i = 0; i < l; i++) {
      ch = text.charCodeAt(i);
      if (Math.random() > 0.5) {
        ch = 'x' + ch.toString(16);
      }
      out += '&#' + ch + ';';
    }

    return out;
  }

  /**
   * Block Lexer
   */
  var Lexer = /*@__PURE__*/(function () {
    function Lexer(options) {
      this.tokens = [];
      this.tokens.links = Object.create(null);
      this.options = options || defaults$2;
      this.options.tokenizer = this.options.tokenizer || new Tokenizer();
      this.tokenizer = this.options.tokenizer;
      this.tokenizer.options = this.options;

      var rules = {
        block: block$1.normal,
        inline: inline$1.normal
      };

      if (this.options.pedantic) {
        rules.block = block$1.pedantic;
        rules.inline = inline$1.pedantic;
      } else if (this.options.gfm) {
        rules.block = block$1.gfm;
        if (this.options.breaks) {
          rules.inline = inline$1.breaks;
        } else {
          rules.inline = inline$1.gfm;
        }
      }
      this.tokenizer.rules = rules;
    }

    var staticAccessors = { rules: { configurable: true } };

    /**
     * Expose Rules
     */
    staticAccessors.rules.get = function () {
      return {
        block: block$1,
        inline: inline$1
      };
    };

    /**
     * Static Lex Method
     */
    Lexer.lex = function lex (src, options) {
      var lexer = new Lexer(options);
      return lexer.lex(src);
    };

    /**
     * Static Lex Inline Method
     */
    Lexer.lexInline = function lexInline (src, options) {
      var lexer = new Lexer(options);
      return lexer.inlineTokens(src);
    };

    /**
     * Preprocessing
     */
    Lexer.prototype.lex = function lex (src) {
      src = src
        .replace(/\r\n|\r/g, '\n')
        .replace(/\t/g, '    ');

      this.blockTokens(src, this.tokens, true);

      this.inline(this.tokens);

      return this.tokens;
    };

    /**
     * Lexing
     */
    Lexer.prototype.blockTokens = function blockTokens (src, tokens, top) {
      if ( tokens === void 0 ) tokens = [];
      if ( top === void 0 ) top = true;

      if (this.options.pedantic) {
        src = src.replace(/^ +$/gm, '');
      }
      var token, i, l, lastToken;

      while (src) {
        // newline
        if (token = this.tokenizer.space(src)) {
          src = src.substring(token.raw.length);
          if (token.type) {
            tokens.push(token);
          }
          continue;
        }

        // code
        if (token = this.tokenizer.code(src, tokens)) {
          src = src.substring(token.raw.length);
          if (token.type) {
            tokens.push(token);
          } else {
            lastToken = tokens[tokens.length - 1];
            lastToken.raw += '\n' + token.raw;
            lastToken.text += '\n' + token.text;
          }
          continue;
        }

        // fences
        if (token = this.tokenizer.fences(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // heading
        if (token = this.tokenizer.heading(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // table no leading pipe (gfm)
        if (token = this.tokenizer.nptable(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // hr
        if (token = this.tokenizer.hr(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // blockquote
        if (token = this.tokenizer.blockquote(src)) {
          src = src.substring(token.raw.length);
          token.tokens = this.blockTokens(token.text, [], top);
          tokens.push(token);
          continue;
        }

        // list
        if (token = this.tokenizer.list(src)) {
          src = src.substring(token.raw.length);
          l = token.items.length;
          for (i = 0; i < l; i++) {
            token.items[i].tokens = this.blockTokens(token.items[i].text, [], false);
          }
          tokens.push(token);
          continue;
        }

        // html
        if (token = this.tokenizer.html(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // def
        if (top && (token = this.tokenizer.def(src))) {
          src = src.substring(token.raw.length);
          if (!this.tokens.links[token.tag]) {
            this.tokens.links[token.tag] = {
              href: token.href,
              title: token.title
            };
          }
          continue;
        }

        // table (gfm)
        if (token = this.tokenizer.table(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // lheading
        if (token = this.tokenizer.lheading(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // top-level paragraph
        if (top && (token = this.tokenizer.paragraph(src))) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // text
        if (token = this.tokenizer.text(src, tokens)) {
          src = src.substring(token.raw.length);
          if (token.type) {
            tokens.push(token);
          } else {
            lastToken = tokens[tokens.length - 1];
            lastToken.raw += '\n' + token.raw;
            lastToken.text += '\n' + token.text;
          }
          continue;
        }

        if (src) {
          var errMsg = 'Infinite loop on byte: ' + src.charCodeAt(0);
          if (this.options.silent) {
            console.error(errMsg);
            break;
          } else {
            throw new Error(errMsg);
          }
        }
      }

      return tokens;
    };

    Lexer.prototype.inline = function inline (tokens) {
      var i,
        j,
        k,
        l2,
        row,
        token;

      var l = tokens.length;
      for (i = 0; i < l; i++) {
        token = tokens[i];
        switch (token.type) {
          case 'paragraph':
          case 'text':
          case 'heading': {
            token.tokens = [];
            this.inlineTokens(token.text, token.tokens);
            break;
          }
          case 'table': {
            token.tokens = {
              header: [],
              cells: []
            };

            // header
            l2 = token.header.length;
            for (j = 0; j < l2; j++) {
              token.tokens.header[j] = [];
              this.inlineTokens(token.header[j], token.tokens.header[j]);
            }

            // cells
            l2 = token.cells.length;
            for (j = 0; j < l2; j++) {
              row = token.cells[j];
              token.tokens.cells[j] = [];
              for (k = 0; k < row.length; k++) {
                token.tokens.cells[j][k] = [];
                this.inlineTokens(row[k], token.tokens.cells[j][k]);
              }
            }

            break;
          }
          case 'blockquote': {
            this.inline(token.tokens);
            break;
          }
          case 'list': {
            l2 = token.items.length;
            for (j = 0; j < l2; j++) {
              this.inline(token.items[j].tokens);
            }
            break;
          }
        }
      }

      return tokens;
    };

    /**
     * Lexing/Compiling
     */
    Lexer.prototype.inlineTokens = function inlineTokens (src, tokens, inLink, inRawBlock) {
      if ( tokens === void 0 ) tokens = [];
      if ( inLink === void 0 ) inLink = false;
      if ( inRawBlock === void 0 ) inRawBlock = false;

      var token;

      // String with links masked to avoid interference with em and strong
      var maskedSrc = src;
      var match;
      var keepPrevChar, prevChar;

      // Mask out reflinks
      if (this.tokens.links) {
        var links = Object.keys(this.tokens.links);
        if (links.length > 0) {
          while ((match = this.tokenizer.rules.inline.reflinkSearch.exec(maskedSrc)) != null) {
            if (links.includes(match[0].slice(match[0].lastIndexOf('[') + 1, -1))) {
              maskedSrc = maskedSrc.slice(0, match.index) + '[' + repeatString$1('a', match[0].length - 2) + ']' + maskedSrc.slice(this.tokenizer.rules.inline.reflinkSearch.lastIndex);
            }
          }
        }
      }
      // Mask out other blocks
      while ((match = this.tokenizer.rules.inline.blockSkip.exec(maskedSrc)) != null) {
        maskedSrc = maskedSrc.slice(0, match.index) + '[' + repeatString$1('a', match[0].length - 2) + ']' + maskedSrc.slice(this.tokenizer.rules.inline.blockSkip.lastIndex);
      }

      while (src) {
        if (!keepPrevChar) {
          prevChar = '';
        }
        keepPrevChar = false;
        // escape
        if (token = this.tokenizer.escape(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // tag
        if (token = this.tokenizer.tag(src, inLink, inRawBlock)) {
          src = src.substring(token.raw.length);
          inLink = token.inLink;
          inRawBlock = token.inRawBlock;
          tokens.push(token);
          continue;
        }

        // link
        if (token = this.tokenizer.link(src)) {
          src = src.substring(token.raw.length);
          if (token.type === 'link') {
            token.tokens = this.inlineTokens(token.text, [], true, inRawBlock);
          }
          tokens.push(token);
          continue;
        }

        // reflink, nolink
        if (token = this.tokenizer.reflink(src, this.tokens.links)) {
          src = src.substring(token.raw.length);
          if (token.type === 'link') {
            token.tokens = this.inlineTokens(token.text, [], true, inRawBlock);
          }
          tokens.push(token);
          continue;
        }

        // strong
        if (token = this.tokenizer.strong(src, maskedSrc, prevChar)) {
          src = src.substring(token.raw.length);
          token.tokens = this.inlineTokens(token.text, [], inLink, inRawBlock);
          tokens.push(token);
          continue;
        }

        // em
        if (token = this.tokenizer.em(src, maskedSrc, prevChar)) {
          src = src.substring(token.raw.length);
          token.tokens = this.inlineTokens(token.text, [], inLink, inRawBlock);
          tokens.push(token);
          continue;
        }

        // code
        if (token = this.tokenizer.codespan(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // br
        if (token = this.tokenizer.br(src)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // del (gfm)
        if (token = this.tokenizer.del(src)) {
          src = src.substring(token.raw.length);
          token.tokens = this.inlineTokens(token.text, [], inLink, inRawBlock);
          tokens.push(token);
          continue;
        }

        // autolink
        if (token = this.tokenizer.autolink(src, mangle)) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // url (gfm)
        if (!inLink && (token = this.tokenizer.url(src, mangle))) {
          src = src.substring(token.raw.length);
          tokens.push(token);
          continue;
        }

        // text
        if (token = this.tokenizer.inlineText(src, inRawBlock, smartypants)) {
          src = src.substring(token.raw.length);
          prevChar = token.raw.slice(-1);
          keepPrevChar = true;
          tokens.push(token);
          continue;
        }

        if (src) {
          var errMsg = 'Infinite loop on byte: ' + src.charCodeAt(0);
          if (this.options.silent) {
            console.error(errMsg);
            break;
          } else {
            throw new Error(errMsg);
          }
        }
      }

      return tokens;
    };

    Object.defineProperties( Lexer, staticAccessors );

    return Lexer;
  }());

  var defaults$3 = defaults.defaults;

  var cleanUrl$1 = helpers.cleanUrl;
  var escape$2 = helpers.escape;

  /**
   * Renderer
   */
  var Renderer = /*@__PURE__*/(function () {
    function Renderer(options) {
      this.options = options || defaults$3;
    }

    Renderer.prototype.code = function code (code$1, infostring, escaped) {
      var lang = (infostring || '').match(/\S*/)[0];
      if (this.options.highlight) {
        var out = this.options.highlight(code$1, lang);
        if (out != null && out !== code$1) {
          escaped = true;
          code$1 = out;
        }
      }

      code$1 = code$1.replace(/\n$/, '') + '\n';

      if (!lang) {
        return '<pre><code>'
          + (escaped ? code$1 : escape$2(code$1, true))
          + '</code></pre>\n';
      }

      return '<pre><code class="'
        + this.options.langPrefix
        + escape$2(lang, true)
        + '">'
        + (escaped ? code$1 : escape$2(code$1, true))
        + '</code></pre>\n';
    };

    Renderer.prototype.blockquote = function blockquote (quote) {
      return '<blockquote>\n' + quote + '</blockquote>\n';
    };

    Renderer.prototype.html = function html (html$1) {
      return html$1;
    };

    Renderer.prototype.heading = function heading (text, level, raw, slugger) {
      if (this.options.headerIds) {
        return '<h'
          + level
          + ' id="'
          + this.options.headerPrefix
          + slugger.slug(raw)
          + '">'
          + text
          + '</h'
          + level
          + '>\n';
      }
      // ignore IDs
      return '<h' + level + '>' + text + '</h' + level + '>\n';
    };

    Renderer.prototype.hr = function hr () {
      return this.options.xhtml ? '<hr/>\n' : '<hr>\n';
    };

    Renderer.prototype.list = function list (body, ordered, start) {
      var type = ordered ? 'ol' : 'ul',
        startatt = (ordered && start !== 1) ? (' start="' + start + '"') : '';
      return '<' + type + startatt + '>\n' + body + '</' + type + '>\n';
    };

    Renderer.prototype.listitem = function listitem (text) {
      return '<li>' + text + '</li>\n';
    };

    Renderer.prototype.checkbox = function checkbox (checked) {
      return '<input '
        + (checked ? 'checked="" ' : '')
        + 'disabled="" type="checkbox"'
        + (this.options.xhtml ? ' /' : '')
        + '> ';
    };

    Renderer.prototype.paragraph = function paragraph (text) {
      return '<p>' + text + '</p>\n';
    };

    Renderer.prototype.table = function table (header, body) {
      if (body) { body = '<tbody>' + body + '</tbody>'; }

      return '<table>\n'
        + '<thead>\n'
        + header
        + '</thead>\n'
        + body
        + '</table>\n';
    };

    Renderer.prototype.tablerow = function tablerow (content) {
      return '<tr>\n' + content + '</tr>\n';
    };

    Renderer.prototype.tablecell = function tablecell (content, flags) {
      var type = flags.header ? 'th' : 'td';
      var tag = flags.align
        ? '<' + type + ' align="' + flags.align + '">'
        : '<' + type + '>';
      return tag + content + '</' + type + '>\n';
    };

    // span level renderer
    Renderer.prototype.strong = function strong (text) {
      return '<strong>' + text + '</strong>';
    };

    Renderer.prototype.em = function em (text) {
      return '<em>' + text + '</em>';
    };

    Renderer.prototype.codespan = function codespan (text) {
      return '<code>' + text + '</code>';
    };

    Renderer.prototype.br = function br () {
      return this.options.xhtml ? '<br/>' : '<br>';
    };

    Renderer.prototype.del = function del (text) {
      return '<del>' + text + '</del>';
    };

    Renderer.prototype.link = function link (href, title, text) {
      href = cleanUrl$1(this.options.sanitize, this.options.baseUrl, href);
      if (href === null) {
        return text;
      }
      var out = '<a href="' + escape$2(href) + '"';
      if (title) {
        out += ' title="' + title + '"';
      }
      out += '>' + text + '</a>';
      return out;
    };

    Renderer.prototype.image = function image (href, title, text) {
      href = cleanUrl$1(this.options.sanitize, this.options.baseUrl, href);
      if (href === null) {
        return text;
      }

      var out = '<img src="' + href + '" alt="' + text + '"';
      if (title) {
        out += ' title="' + title + '"';
      }
      out += this.options.xhtml ? '/>' : '>';
      return out;
    };

    Renderer.prototype.text = function text (text$1) {
      return text$1;
    };

    return Renderer;
  }());

  /**
   * TextRenderer
   * returns only the textual part of the token
   */
  var TextRenderer = /*@__PURE__*/(function () {
    function TextRenderer () {}

    TextRenderer.prototype.strong = function strong (text) {
      return text;
    };

    TextRenderer.prototype.em = function em (text) {
      return text;
    };

    TextRenderer.prototype.codespan = function codespan (text) {
      return text;
    };

    TextRenderer.prototype.del = function del (text) {
      return text;
    };

    TextRenderer.prototype.html = function html (text) {
      return text;
    };

    TextRenderer.prototype.text = function text (text$1) {
      return text$1;
    };

    TextRenderer.prototype.link = function link (href, title, text) {
      return '' + text;
    };

    TextRenderer.prototype.image = function image (href, title, text) {
      return '' + text;
    };

    TextRenderer.prototype.br = function br () {
      return '';
    };

    return TextRenderer;
  }());

  /**
   * Slugger generates header id
   */
  var Slugger = /*@__PURE__*/(function () {
    function Slugger() {
      this.seen = {};
    }

    Slugger.prototype.serialize = function serialize (value) {
      return value
        .toLowerCase()
        .trim()
        // remove html tags
        .replace(/<[!\/a-z].*?>/ig, '')
        // remove unwanted chars
        .replace(/[\u2000-\u206F\u2E00-\u2E7F\\'!"#$%&()*+,./:;<=>?@[\]^`{|}~]/g, '')
        .replace(/\s/g, '-');
    };

    /**
     * Finds the next safe (unique) slug to use
     */
    Slugger.prototype.getNextSafeSlug = function getNextSafeSlug (originalSlug, isDryRun) {
      var slug = originalSlug;
      var occurenceAccumulator = 0;
      if (this.seen.hasOwnProperty(slug)) {
        occurenceAccumulator = this.seen[originalSlug];
        do {
          occurenceAccumulator++;
          slug = originalSlug + '-' + occurenceAccumulator;
        } while (this.seen.hasOwnProperty(slug));
      }
      if (!isDryRun) {
        this.seen[originalSlug] = occurenceAccumulator;
        this.seen[slug] = 0;
      }
      return slug;
    };

    /**
     * Convert string to unique id
     * @param {object} options
     * @param {boolean} options.dryrun Generates the next unique slug without updating the internal accumulator.
     */
    Slugger.prototype.slug = function slug (value, options) {
      if ( options === void 0 ) options = {};

      var slug = this.serialize(value);
      return this.getNextSafeSlug(slug, options.dryrun);
    };

    return Slugger;
  }());

  var defaults$4 = defaults.defaults;

  var unescape$1 = helpers.unescape;

  /**
   * Parsing & Compiling
   */
  var Parser = /*@__PURE__*/(function () {
    function Parser(options) {
      this.options = options || defaults$4;
      this.options.renderer = this.options.renderer || new Renderer();
      this.renderer = this.options.renderer;
      this.renderer.options = this.options;
      this.textRenderer = new TextRenderer();
      this.slugger = new Slugger();
    }

    /**
     * Static Parse Method
     */
    Parser.parse = function parse (tokens, options) {
      var parser = new Parser(options);
      return parser.parse(tokens);
    };

    /**
     * Static Parse Inline Method
     */
    Parser.parseInline = function parseInline (tokens, options) {
      var parser = new Parser(options);
      return parser.parseInline(tokens);
    };

    /**
     * Parse Loop
     */
    Parser.prototype.parse = function parse (tokens, top) {
      if ( top === void 0 ) top = true;

      var out = '',
        i,
        j,
        k,
        l2,
        l3,
        row,
        cell,
        header,
        body,
        token,
        ordered,
        start,
        loose,
        itemBody,
        item,
        checked,
        task,
        checkbox;

      var l = tokens.length;
      for (i = 0; i < l; i++) {
        token = tokens[i];
        switch (token.type) {
          case 'space': {
            continue;
          }
          case 'hr': {
            out += this.renderer.hr();
            continue;
          }
          case 'heading': {
            out += this.renderer.heading(
              this.parseInline(token.tokens),
              token.depth,
              unescape$1(this.parseInline(token.tokens, this.textRenderer)),
              this.slugger);
            continue;
          }
          case 'code': {
            out += this.renderer.code(token.text,
              token.lang,
              token.escaped);
            continue;
          }
          case 'table': {
            header = '';

            // header
            cell = '';
            l2 = token.header.length;
            for (j = 0; j < l2; j++) {
              cell += this.renderer.tablecell(
                this.parseInline(token.tokens.header[j]),
                { header: true, align: token.align[j] }
              );
            }
            header += this.renderer.tablerow(cell);

            body = '';
            l2 = token.cells.length;
            for (j = 0; j < l2; j++) {
              row = token.tokens.cells[j];

              cell = '';
              l3 = row.length;
              for (k = 0; k < l3; k++) {
                cell += this.renderer.tablecell(
                  this.parseInline(row[k]),
                  { header: false, align: token.align[k] }
                );
              }

              body += this.renderer.tablerow(cell);
            }
            out += this.renderer.table(header, body);
            continue;
          }
          case 'blockquote': {
            body = this.parse(token.tokens);
            out += this.renderer.blockquote(body);
            continue;
          }
          case 'list': {
            ordered = token.ordered;
            start = token.start;
            loose = token.loose;
            l2 = token.items.length;

            body = '';
            for (j = 0; j < l2; j++) {
              item = token.items[j];
              checked = item.checked;
              task = item.task;

              itemBody = '';
              if (item.task) {
                checkbox = this.renderer.checkbox(checked);
                if (loose) {
                  if (item.tokens.length > 0 && item.tokens[0].type === 'text') {
                    item.tokens[0].text = checkbox + ' ' + item.tokens[0].text;
                    if (item.tokens[0].tokens && item.tokens[0].tokens.length > 0 && item.tokens[0].tokens[0].type === 'text') {
                      item.tokens[0].tokens[0].text = checkbox + ' ' + item.tokens[0].tokens[0].text;
                    }
                  } else {
                    item.tokens.unshift({
                      type: 'text',
                      text: checkbox
                    });
                  }
                } else {
                  itemBody += checkbox;
                }
              }

              itemBody += this.parse(item.tokens, loose);
              body += this.renderer.listitem(itemBody, task, checked);
            }

            out += this.renderer.list(body, ordered, start);
            continue;
          }
          case 'html': {
            // TODO parse inline content if parameter markdown=1
            out += this.renderer.html(token.text);
            continue;
          }
          case 'paragraph': {
            out += this.renderer.paragraph(this.parseInline(token.tokens));
            continue;
          }
          case 'text': {
            body = token.tokens ? this.parseInline(token.tokens) : token.text;
            while (i + 1 < l && tokens[i + 1].type === 'text') {
              token = tokens[++i];
              body += '\n' + (token.tokens ? this.parseInline(token.tokens) : token.text);
            }
            out += top ? this.renderer.paragraph(body) : body;
            continue;
          }
          default: {
            var errMsg = 'Token with "' + token.type + '" type was not found.';
            if (this.options.silent) {
              console.error(errMsg);
              return;
            } else {
              throw new Error(errMsg);
            }
          }
        }
      }

      return out;
    };

    /**
     * Parse Inline Tokens
     */
    Parser.prototype.parseInline = function parseInline (tokens, renderer) {
      renderer = renderer || this.renderer;
      var out = '',
        i,
        token;

      var l = tokens.length;
      for (i = 0; i < l; i++) {
        token = tokens[i];
        switch (token.type) {
          case 'escape': {
            out += renderer.text(token.text);
            break;
          }
          case 'html': {
            out += renderer.html(token.text);
            break;
          }
          case 'link': {
            out += renderer.link(token.href, token.title, this.parseInline(token.tokens, renderer));
            break;
          }
          case 'image': {
            out += renderer.image(token.href, token.title, token.text);
            break;
          }
          case 'strong': {
            out += renderer.strong(this.parseInline(token.tokens, renderer));
            break;
          }
          case 'em': {
            out += renderer.em(this.parseInline(token.tokens, renderer));
            break;
          }
          case 'codespan': {
            out += renderer.codespan(token.text);
            break;
          }
          case 'br': {
            out += renderer.br();
            break;
          }
          case 'del': {
            out += renderer.del(this.parseInline(token.tokens, renderer));
            break;
          }
          case 'text': {
            out += renderer.text(token.text);
            break;
          }
          default: {
            var errMsg = 'Token with "' + token.type + '" type was not found.';
            if (this.options.silent) {
              console.error(errMsg);
              return;
            } else {
              throw new Error(errMsg);
            }
          }
        }
      }
      return out;
    };

    return Parser;
  }());

  var merge$3 = helpers.merge;
  var checkSanitizeDeprecation$1 = helpers.checkSanitizeDeprecation;
  var escape$3 = helpers.escape;

  var getDefaults = defaults.getDefaults;
  var changeDefaults = defaults.changeDefaults;
  var defaults$5 = defaults.defaults;

  /**
   * Marked
   */
  function marked(src, opt, callback) {
    // throw error in case of non string input
    if (typeof src === 'undefined' || src === null) {
      throw new Error('marked(): input parameter is undefined or null');
    }
    if (typeof src !== 'string') {
      throw new Error('marked(): input parameter is of type '
        + Object.prototype.toString.call(src) + ', string expected');
    }

    if (typeof opt === 'function') {
      callback = opt;
      opt = null;
    }

    opt = merge$3({}, marked.defaults, opt || {});
    checkSanitizeDeprecation$1(opt);

    if (callback) {
      var highlight = opt.highlight;
      var tokens;

      try {
        tokens = Lexer.lex(src, opt);
      } catch (e) {
        return callback(e);
      }

      var done = function(err) {
        var out;

        if (!err) {
          try {
            out = Parser.parse(tokens, opt);
          } catch (e) {
            err = e;
          }
        }

        opt.highlight = highlight;

        return err
          ? callback(err)
          : callback(null, out);
      };

      if (!highlight || highlight.length < 3) {
        return done();
      }

      delete opt.highlight;

      if (!tokens.length) { return done(); }

      var pending = 0;
      marked.walkTokens(tokens, function(token) {
        if (token.type === 'code') {
          pending++;
          setTimeout(function () {
            highlight(token.text, token.lang, function(err, code) {
              if (err) {
                return done(err);
              }
              if (code != null && code !== token.text) {
                token.text = code;
                token.escaped = true;
              }

              pending--;
              if (pending === 0) {
                done();
              }
            });
          }, 0);
        }
      });

      if (pending === 0) {
        done();
      }

      return;
    }

    try {
      var tokens$1 = Lexer.lex(src, opt);
      if (opt.walkTokens) {
        marked.walkTokens(tokens$1, opt.walkTokens);
      }
      return Parser.parse(tokens$1, opt);
    } catch (e) {
      e.message += '\nPlease report this to https://github.com/markedjs/marked.';
      if (opt.silent) {
        return '<p>An error occurred:</p><pre>'
          + escape$3(e.message + '', true)
          + '</pre>';
      }
      throw e;
    }
  }

  /**
   * Options
   */

  marked.options =
  marked.setOptions = function(opt) {
    merge$3(marked.defaults, opt);
    changeDefaults(marked.defaults);
    return marked;
  };

  marked.getDefaults = getDefaults;

  marked.defaults = defaults$5;

  /**
   * Use Extension
   */

  marked.use = function(extension) {
    var opts = merge$3({}, extension);
    if (extension.renderer) {
      var renderer = marked.defaults.renderer || new Renderer();
      var loop = function ( prop ) {
        var prevRenderer = renderer[prop];
        renderer[prop] = function () {
          var args = [], len = arguments.length;
          while ( len-- ) args[ len ] = arguments[ len ];

          var ret = extension.renderer[prop].apply(renderer, args);
          if (ret === false) {
            ret = prevRenderer.apply(renderer, args);
          }
          return ret;
        };
      };

      for (var prop in extension.renderer) loop( prop );
      opts.renderer = renderer;
    }
    if (extension.tokenizer) {
      var tokenizer = marked.defaults.tokenizer || new Tokenizer();
      var loop$1 = function ( prop ) {
        var prevTokenizer = tokenizer[prop$1];
        tokenizer[prop$1] = function () {
          var args = [], len = arguments.length;
          while ( len-- ) args[ len ] = arguments[ len ];

          var ret = extension.tokenizer[prop$1].apply(tokenizer, args);
          if (ret === false) {
            ret = prevTokenizer.apply(tokenizer, args);
          }
          return ret;
        };
      };

      for (var prop$1 in extension.tokenizer) loop$1( prop );
      opts.tokenizer = tokenizer;
    }
    if (extension.walkTokens) {
      var walkTokens = marked.defaults.walkTokens;
      opts.walkTokens = function (token) {
        extension.walkTokens(token);
        if (walkTokens) {
          walkTokens(token);
        }
      };
    }
    marked.setOptions(opts);
  };

  /**
   * Run callback for every token
   */

  marked.walkTokens = function(tokens, callback) {
    for (var i$3 = 0, list$3 = tokens; i$3 < list$3.length; i$3 += 1) {
      var token = list$3[i$3];

      callback(token);
      switch (token.type) {
        case 'table': {
          for (var i = 0, list = token.tokens.header; i < list.length; i += 1) {
            var cell = list[i];

            marked.walkTokens(cell, callback);
          }
          for (var i$2 = 0, list$2 = token.tokens.cells; i$2 < list$2.length; i$2 += 1) {
            var row = list$2[i$2];

            for (var i$1 = 0, list$1 = row; i$1 < list$1.length; i$1 += 1) {
              var cell$1 = list$1[i$1];

              marked.walkTokens(cell$1, callback);
            }
          }
          break;
        }
        case 'list': {
          marked.walkTokens(token.items, callback);
          break;
        }
        default: {
          if (token.tokens) {
            marked.walkTokens(token.tokens, callback);
          }
        }
      }
    }
  };

  /**
   * Parse Inline
   */
  marked.parseInline = function(src, opt) {
    // throw error in case of non string input
    if (typeof src === 'undefined' || src === null) {
      throw new Error('marked.parseInline(): input parameter is undefined or null');
    }
    if (typeof src !== 'string') {
      throw new Error('marked.parseInline(): input parameter is of type '
        + Object.prototype.toString.call(src) + ', string expected');
    }

    opt = merge$3({}, marked.defaults, opt || {});
    checkSanitizeDeprecation$1(opt);

    try {
      var tokens = Lexer.lexInline(src, opt);
      if (opt.walkTokens) {
        marked.walkTokens(tokens, opt.walkTokens);
      }
      return Parser.parseInline(tokens, opt);
    } catch (e) {
      e.message += '\nPlease report this to https://github.com/markedjs/marked.';
      if (opt.silent) {
        return '<p>An error occurred:</p><pre>'
          + escape$3(e.message + '', true)
          + '</pre>';
      }
      throw e;
    }
  };

  /**
   * Expose
   */

  marked.Parser = Parser;
  marked.parser = Parser.parse;

  marked.Renderer = Renderer;
  marked.TextRenderer = TextRenderer;

  marked.Lexer = Lexer;
  marked.lexer = Lexer.lex;

  marked.Tokenizer = Tokenizer;

  marked.Slugger = Slugger;

  marked.parse = marked;

  var marked_1 = marked;

  /**
   * Render github corner
   * @param  {Object} data URL for the View Source on Github link
   * @param {String} cornerExternalLinkTarge value of the target attribute of the link
   * @return {String} SVG element as string
   */
  function corner(data, cornerExternalLinkTarge) {
    if (!data) {
      return '';
    }

    if (!/\/\//.test(data)) {
      data = 'https://github.com/' + data;
    }

    data = data.replace(/^git\+/, '');
    // Double check
    cornerExternalLinkTarge = cornerExternalLinkTarge || '_blank';

    return (
      "<a href=\"" + data + "\" target=\"" + cornerExternalLinkTarge + "\" class=\"github-corner\" aria-label=\"View source on Github\">" +
      '<svg viewBox="0 0 250 250" aria-hidden="true">' +
      '<path d="M0,0 L115,115 L130,115 L142,142 L250,250 L250,0 Z"></path>' +
      '<path d="M128.3,109.0 C113.8,99.7 119.0,89.6 119.0,89.6 C122.0,82.7 120.5,78.6 120.5,78.6 C119.2,72.0 123.4,76.3 123.4,76.3 C127.3,80.9 125.5,87.3 125.5,87.3 C122.9,97.6 130.6,101.9 134.4,103.2" fill="currentColor" style="transform-origin: 130px 106px;" class="octo-arm"></path>' +
      '<path d="M115.0,115.0 C114.9,115.1 118.7,116.5 119.8,115.4 L133.7,101.6 C136.9,99.2 139.9,98.4 142.2,98.6 C133.8,88.0 127.5,74.4 143.8,58.0 C148.5,53.4 154.0,51.2 159.7,51.0 C160.3,49.4 163.2,43.6 171.4,40.1 C171.4,40.1 176.1,42.5 178.8,56.2 C183.1,58.6 187.2,61.8 190.9,65.4 C194.5,69.0 197.7,73.2 200.1,77.6 C213.8,80.2 216.3,84.9 216.3,84.9 C212.7,93.1 206.9,96.0 205.4,96.6 C205.1,102.4 203.0,107.8 198.3,112.5 C181.9,128.9 168.3,122.5 157.7,114.1 C157.9,116.9 156.7,120.9 152.7,124.9 L141.0,136.5 C139.8,137.7 141.6,141.9 141.8,141.8 Z" fill="currentColor" class="octo-body"></path>' +
      '</svg>' +
      '</a>'
    );
  }

  /**
   * Renders main content
   * @param {Object} config Configuration object
   * @returns {String} HTML of the main content
   */
  function main(config) {
    var name = config.name ? config.name : '';

    var aside =
      '<button class="sidebar-toggle" aria-label="Menu">' +
      '<div class="sidebar-toggle-button">' +
      '<span></span><span></span><span></span>' +
      '</div>' +
      '</button>' +
      '<aside class="sidebar">' +
      (config.name
        ? ("<h1 class=\"app-name\"><a class=\"app-name-link\" data-nosearch>" + (config.logo ? ("<img alt=\"" + name + "\" src=" + (config.logo) + ">") : name) + "</a></h1>")
        : '') +
      '<div class="sidebar-nav"><!--sidebar--></div>' +
      '</aside>';
    return (
      "<main>" + aside +
      '<section class="content">' +
      '<article class="markdown-section" id="main"><!--main--></article>' +
      '</section>' +
      '</main>'
    );
  }

  /**
   * Cover Page
   * @returns {String} Cover page
   */
  function cover() {
    var SL = ', 100%, 85%';
    var bgc =
      'linear-gradient(to left bottom, ' +
      "hsl(" + (Math.floor(Math.random() * 255) + SL) + ") 0%," +
      "hsl(" + (Math.floor(Math.random() * 255) + SL) + ") 100%)";

    return (
      "<section class=\"cover show\" style=\"background: " + bgc + "\">" +
      '<div class="mask"></div>' +
      '<div class="cover-main"><!--cover--></div>' +
      '</section>'
    );
  }

  /**
   * Render tree
   * @param  {Array} toc Array of TOC section links
   * @param  {String} tpl TPL list
   * @return {String} Rendered tree
   */
  function tree(toc, tpl) {
    if ( tpl === void 0 ) tpl = '<ul class="app-sub-sidebar">{inner}</ul>';

    if (!toc || !toc.length) {
      return '';
    }

    var innerHTML = '';
    toc.forEach(function (node) {
      var title = node.title.replace(/(<([^>]+)>)/g, '');
      innerHTML += "<li><a class=\"section-link\" href=\"" + (node.slug) + "\" title=\"" + title + "\">" + (node.title) + "</a></li>";
      if (node.children) {
        innerHTML += tree(node.children, tpl);
      }
    });
    return tpl.replace('{inner}', innerHTML);
  }

  function helper(className, content) {
    return ("<p class=\"" + className + "\">" + (content.slice(5).trim()) + "</p>");
  }

  function theme(color) {
    return ("<style>:root{--theme-color: " + color + ";}</style>");
  }

  /**
   * Gen toc tree
   * @link https://github.com/killercup/grock/blob/5280ae63e16c5739e9233d9009bc235ed7d79a50/styles/solarized/assets/js/behavior.coffee#L54-L81
   * @param  {Array} toc List of TOC elements
   * @param  {Number} maxLevel Deep level
   * @return {Array} Headlines
   */
  function genTree(toc, maxLevel) {
    var headlines = [];
    var last = {};

    toc.forEach(function (headline) {
      var level = headline.level || 1;
      var len = level - 1;

      if (level > maxLevel) {
        return;
      }

      if (last[len]) {
        last[len].children = (last[len].children || []).concat(headline);
      } else {
        headlines.push(headline);
      }

      last[level] = headline;
    });

    return headlines;
  }

  var cache$1 = {};
  var re = /[\u2000-\u206F\u2E00-\u2E7F\\'!"#$%&()*+,./:;<=>?@[\]^`{|}~]/g;

  function lower(string) {
    return string.toLowerCase();
  }

  function slugify(str) {
    if (typeof str !== 'string') {
      return '';
    }

    var slug = str
      .trim()
      .replace(/[A-Z]+/g, lower)
      .replace(/<[^>]+>/g, '')
      .replace(re, '')
      .replace(/\s/g, '-')
      .replace(/-+/g, '-')
      .replace(/^(\d)/, '_$1');
    var count = cache$1[slug];

    count = hasOwn.call(cache$1, slug) ? count + 1 : 0;
    cache$1[slug] = count;

    if (count) {
      slug = slug + '-' + count;
    }

    return slug;
  }

  slugify.clear = function() {
    cache$1 = {};
  };

  function replace(m, $1) {
    return (
      '<img class="emoji" src="https://github.githubassets.com/images/icons/emoji/' +
      $1 +
      '.png" alt="' +
      $1 +
      '" />'
    );
  }

  function emojify(text) {
    return text
      .replace(/:\+1:/g, ':thumbsup:')
      .replace(/:-1:/g, ':thumbsdown:')
      .replace(/<(pre|template|code)[^>]*?>[\s\S]+?<\/(pre|template|code)>/g, function (m) { return m.replace(/:/g, '__colon__'); }
      )
      .replace(/:(\w+?):/gi, ( window.emojify) || replace)
      .replace(/__colon__/g, ':');
  }

  /**
   * Converts a colon formatted string to a object with properties.
   *
   * This is process a provided string and look for any tokens in the format
   * of `:name[=value]` and then convert it to a object and return.
   * An example of this is ':include :type=code :fragment=demo' is taken and
   * then converted to:
   *
   * ```
   * {
   *  include: '',
   *  type: 'code',
   *  fragment: 'demo'
   * }
   * ```
   *
   * @param {string}   str   The string to parse.
   *
   * @return {object}  The original string and parsed object, { str, config }.
   */
  function getAndRemoveConfig(str) {
    if ( str === void 0 ) str = '';

    var config = {};

    if (str) {
      str = str
        .replace(/^('|")/, '')
        .replace(/('|")$/, '')
        .replace(/(?:^|\s):([\w-]+:?)=?([\w-%]+)?/g, function (m, key, value) {
          if (key.indexOf(':') === -1) {
            config[key] = (value && value.replace(/&quot;/g, '')) || true;
            return '';
          }

          return m;
        })
        .trim();
    }

    return { str: str, config: config };
  }

  /**
   * Remove the <a> tag from sidebar when the header with link, details see issue 1069
   * @param {string}   str   The string to deal with.
   *
   * @return {string}   str   The string after delete the <a> element.
   */
  function removeAtag(str) {
    if ( str === void 0 ) str = '';

    return str.replace(/(<\/?a.*?>)/gi, '');
  }

  var imageCompiler = function (ref) {
      var renderer = ref.renderer;
      var contentBase = ref.contentBase;
      var router = ref.router;

      return (renderer.image = function (href, title, text) {
      var url = href;
      var attrs = [];

      var ref = getAndRemoveConfig(title);
      var str = ref.str;
      var config = ref.config;
      title = str;

      if (config['no-zoom']) {
        attrs.push('data-no-zoom');
      }

      if (title) {
        attrs.push(("title=\"" + title + "\""));
      }

      if (config.size) {
        var ref$1 = config.size.split('x');
        var width = ref$1[0];
        var height = ref$1[1];
        if (height) {
          attrs.push(("width=\"" + width + "\" height=\"" + height + "\""));
        } else {
          attrs.push(("width=\"" + width + "\""));
        }
      }

      if (config.class) {
        attrs.push(("class=\"" + (config.class) + "\""));
      }

      if (config.id) {
        attrs.push(("id=\"" + (config.id) + "\""));
      }

      if (!isAbsolutePath(href)) {
        url = getPath(contentBase, getParentPath(router.getCurrentPath()), href);
      }

      if (attrs.length > 0) {
        return ("<img src=\"" + url + "\" data-origin=\"" + href + "\" alt=\"" + text + "\" " + (attrs.join(
          ' '
        )) + " />");
      }

      return ("<img src=\"" + url + "\" data-origin=\"" + href + "\" alt=\"" + text + "\"" + attrs + ">");
    });
  };

  var prism = createCommonjsModule(function (module) {
  /* **********************************************
       Begin prism-core.js
  ********************************************** */

  /// <reference lib="WebWorker"/>

  var _self = (typeof window !== 'undefined')
  	? window   // if in browser
  	: (
  		(typeof WorkerGlobalScope !== 'undefined' && self instanceof WorkerGlobalScope)
  		? self // if in worker
  		: {}   // if in node js
  	);

  /**
   * Prism: Lightweight, robust, elegant syntax highlighting
   *
   * @license MIT <https://opensource.org/licenses/MIT>
   * @author Lea Verou <https://lea.verou.me>
   * @namespace
   * @public
   */
  var Prism = (function (_self){

  // Private helper vars
  var lang = /\blang(?:uage)?-([\w-]+)\b/i;
  var uniqueId = 0;


  var _ = {
  	/**
  	 * By default, Prism will attempt to highlight all code elements (by calling {@link Prism.highlightAll}) on the
  	 * current page after the page finished loading. This might be a problem if e.g. you wanted to asynchronously load
  	 * additional languages or plugins yourself.
  	 *
  	 * By setting this value to `true`, Prism will not automatically highlight all code elements on the page.
  	 *
  	 * You obviously have to change this value before the automatic highlighting started. To do this, you can add an
  	 * empty Prism object into the global scope before loading the Prism script like this:
  	 *
  	 * ```js
  	 * window.Prism = window.Prism || {};
  	 * Prism.manual = true;
  	 * // add a new <script> to load Prism's script
  	 * ```
  	 *
  	 * @default false
  	 * @type {boolean}
  	 * @memberof Prism
  	 * @public
  	 */
  	manual: _self.Prism && _self.Prism.manual,
  	disableWorkerMessageHandler: _self.Prism && _self.Prism.disableWorkerMessageHandler,

  	/**
  	 * A namespace for utility methods.
  	 *
  	 * All function in this namespace that are not explicitly marked as _public_ are for __internal use only__ and may
  	 * change or disappear at any time.
  	 *
  	 * @namespace
  	 * @memberof Prism
  	 */
  	util: {
  		encode: function encode(tokens) {
  			if (tokens instanceof Token) {
  				return new Token(tokens.type, encode(tokens.content), tokens.alias);
  			} else if (Array.isArray(tokens)) {
  				return tokens.map(encode);
  			} else {
  				return tokens.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/\u00a0/g, ' ');
  			}
  		},

  		/**
  		 * Returns the name of the type of the given value.
  		 *
  		 * @param {any} o
  		 * @returns {string}
  		 * @example
  		 * type(null)      === 'Null'
  		 * type(undefined) === 'Undefined'
  		 * type(123)       === 'Number'
  		 * type('foo')     === 'String'
  		 * type(true)      === 'Boolean'
  		 * type([1, 2])    === 'Array'
  		 * type({})        === 'Object'
  		 * type(String)    === 'Function'
  		 * type(/abc+/)    === 'RegExp'
  		 */
  		type: function (o) {
  			return Object.prototype.toString.call(o).slice(8, -1);
  		},

  		/**
  		 * Returns a unique number for the given object. Later calls will still return the same number.
  		 *
  		 * @param {Object} obj
  		 * @returns {number}
  		 */
  		objId: function (obj) {
  			if (!obj['__id']) {
  				Object.defineProperty(obj, '__id', { value: ++uniqueId });
  			}
  			return obj['__id'];
  		},

  		/**
  		 * Creates a deep clone of the given object.
  		 *
  		 * The main intended use of this function is to clone language definitions.
  		 *
  		 * @param {T} o
  		 * @param {Record<number, any>} [visited]
  		 * @returns {T}
  		 * @template T
  		 */
  		clone: function deepClone(o, visited) {
  			visited = visited || {};

  			var clone, id;
  			switch (_.util.type(o)) {
  				case 'Object':
  					id = _.util.objId(o);
  					if (visited[id]) {
  						return visited[id];
  					}
  					clone = /** @type {Record<string, any>} */ ({});
  					visited[id] = clone;

  					for (var key in o) {
  						if (o.hasOwnProperty(key)) {
  							clone[key] = deepClone(o[key], visited);
  						}
  					}

  					return /** @type {any} */ (clone);

  				case 'Array':
  					id = _.util.objId(o);
  					if (visited[id]) {
  						return visited[id];
  					}
  					clone = [];
  					visited[id] = clone;

  					(/** @type {Array} */(/** @type {any} */(o))).forEach(function (v, i) {
  						clone[i] = deepClone(v, visited);
  					});

  					return /** @type {any} */ (clone);

  				default:
  					return o;
  			}
  		},

  		/**
  		 * Returns the Prism language of the given element set by a `language-xxxx` or `lang-xxxx` class.
  		 *
  		 * If no language is set for the element or the element is `null` or `undefined`, `none` will be returned.
  		 *
  		 * @param {Element} element
  		 * @returns {string}
  		 */
  		getLanguage: function (element) {
  			while (element && !lang.test(element.className)) {
  				element = element.parentElement;
  			}
  			if (element) {
  				return (element.className.match(lang) || [, 'none'])[1].toLowerCase();
  			}
  			return 'none';
  		},

  		/**
  		 * Returns the script element that is currently executing.
  		 *
  		 * This does __not__ work for line script element.
  		 *
  		 * @returns {HTMLScriptElement | null}
  		 */
  		currentScript: function () {
  			if (typeof document === 'undefined') {
  				return null;
  			}
  			if ('currentScript' in document && 1 < 2 /* hack to trip TS' flow analysis */) {
  				return /** @type {any} */ (document.currentScript);
  			}

  			// IE11 workaround
  			// we'll get the src of the current script by parsing IE11's error stack trace
  			// this will not work for inline scripts

  			try {
  				throw new Error();
  			} catch (err) {
  				// Get file src url from stack. Specifically works with the format of stack traces in IE.
  				// A stack will look like this:
  				//
  				// Error
  				//    at _.util.currentScript (http://localhost/components/prism-core.js:119:5)
  				//    at Global code (http://localhost/components/prism-core.js:606:1)

  				var src = (/at [^(\r\n]*\((.*):.+:.+\)$/i.exec(err.stack) || [])[1];
  				if (src) {
  					var scripts = document.getElementsByTagName('script');
  					for (var i in scripts) {
  						if (scripts[i].src == src) {
  							return scripts[i];
  						}
  					}
  				}
  				return null;
  			}
  		},

  		/**
  		 * Returns whether a given class is active for `element`.
  		 *
  		 * The class can be activated if `element` or one of its ancestors has the given class and it can be deactivated
  		 * if `element` or one of its ancestors has the negated version of the given class. The _negated version_ of the
  		 * given class is just the given class with a `no-` prefix.
  		 *
  		 * Whether the class is active is determined by the closest ancestor of `element` (where `element` itself is
  		 * closest ancestor) that has the given class or the negated version of it. If neither `element` nor any of its
  		 * ancestors have the given class or the negated version of it, then the default activation will be returned.
  		 *
  		 * In the paradoxical situation where the closest ancestor contains __both__ the given class and the negated
  		 * version of it, the class is considered active.
  		 *
  		 * @param {Element} element
  		 * @param {string} className
  		 * @param {boolean} [defaultActivation=false]
  		 * @returns {boolean}
  		 */
  		isActive: function (element, className, defaultActivation) {
  			var no = 'no-' + className;

  			while (element) {
  				var classList = element.classList;
  				if (classList.contains(className)) {
  					return true;
  				}
  				if (classList.contains(no)) {
  					return false;
  				}
  				element = element.parentElement;
  			}
  			return !!defaultActivation;
  		}
  	},

  	/**
  	 * This namespace contains all currently loaded languages and the some helper functions to create and modify languages.
  	 *
  	 * @namespace
  	 * @memberof Prism
  	 * @public
  	 */
  	languages: {
  		/**
  		 * Creates a deep copy of the language with the given id and appends the given tokens.
  		 *
  		 * If a token in `redef` also appears in the copied language, then the existing token in the copied language
  		 * will be overwritten at its original position.
  		 *
  		 * ## Best practices
  		 *
  		 * Since the position of overwriting tokens (token in `redef` that overwrite tokens in the copied language)
  		 * doesn't matter, they can technically be in any order. However, this can be confusing to others that trying to
  		 * understand the language definition because, normally, the order of tokens matters in Prism grammars.
  		 *
  		 * Therefore, it is encouraged to order overwriting tokens according to the positions of the overwritten tokens.
  		 * Furthermore, all non-overwriting tokens should be placed after the overwriting ones.
  		 *
  		 * @param {string} id The id of the language to extend. This has to be a key in `Prism.languages`.
  		 * @param {Grammar} redef The new tokens to append.
  		 * @returns {Grammar} The new language created.
  		 * @public
  		 * @example
  		 * Prism.languages['css-with-colors'] = Prism.languages.extend('css', {
  		 *     // Prism.languages.css already has a 'comment' token, so this token will overwrite CSS' 'comment' token
  		 *     // at its original position
  		 *     'comment': { ... },
  		 *     // CSS doesn't have a 'color' token, so this token will be appended
  		 *     'color': /\b(?:red|green|blue)\b/
  		 * });
  		 */
  		extend: function (id, redef) {
  			var lang = _.util.clone(_.languages[id]);

  			for (var key in redef) {
  				lang[key] = redef[key];
  			}

  			return lang;
  		},

  		/**
  		 * Inserts tokens _before_ another token in a language definition or any other grammar.
  		 *
  		 * ## Usage
  		 *
  		 * This helper method makes it easy to modify existing languages. For example, the CSS language definition
  		 * not only defines CSS highlighting for CSS documents, but also needs to define highlighting for CSS embedded
  		 * in HTML through `<style>` elements. To do this, it needs to modify `Prism.languages.markup` and add the
  		 * appropriate tokens. However, `Prism.languages.markup` is a regular JavaScript object literal, so if you do
  		 * this:
  		 *
  		 * ```js
  		 * Prism.languages.markup.style = {
  		 *     // token
  		 * };
  		 * ```
  		 *
  		 * then the `style` token will be added (and processed) at the end. `insertBefore` allows you to insert tokens
  		 * before existing tokens. For the CSS example above, you would use it like this:
  		 *
  		 * ```js
  		 * Prism.languages.insertBefore('markup', 'cdata', {
  		 *     'style': {
  		 *         // token
  		 *     }
  		 * });
  		 * ```
  		 *
  		 * ## Special cases
  		 *
  		 * If the grammars of `inside` and `insert` have tokens with the same name, the tokens in `inside`'s grammar
  		 * will be ignored.
  		 *
  		 * This behavior can be used to insert tokens after `before`:
  		 *
  		 * ```js
  		 * Prism.languages.insertBefore('markup', 'comment', {
  		 *     'comment': Prism.languages.markup.comment,
  		 *     // tokens after 'comment'
  		 * });
  		 * ```
  		 *
  		 * ## Limitations
  		 *
  		 * The main problem `insertBefore` has to solve is iteration order. Since ES2015, the iteration order for object
  		 * properties is guaranteed to be the insertion order (except for integer keys) but some browsers behave
  		 * differently when keys are deleted and re-inserted. So `insertBefore` can't be implemented by temporarily
  		 * deleting properties which is necessary to insert at arbitrary positions.
  		 *
  		 * To solve this problem, `insertBefore` doesn't actually insert the given tokens into the target object.
  		 * Instead, it will create a new object and replace all references to the target object with the new one. This
  		 * can be done without temporarily deleting properties, so the iteration order is well-defined.
  		 *
  		 * However, only references that can be reached from `Prism.languages` or `insert` will be replaced. I.e. if
  		 * you hold the target object in a variable, then the value of the variable will not change.
  		 *
  		 * ```js
  		 * var oldMarkup = Prism.languages.markup;
  		 * var newMarkup = Prism.languages.insertBefore('markup', 'comment', { ... });
  		 *
  		 * assert(oldMarkup !== Prism.languages.markup);
  		 * assert(newMarkup === Prism.languages.markup);
  		 * ```
  		 *
  		 * @param {string} inside The property of `root` (e.g. a language id in `Prism.languages`) that contains the
  		 * object to be modified.
  		 * @param {string} before The key to insert before.
  		 * @param {Grammar} insert An object containing the key-value pairs to be inserted.
  		 * @param {Object<string, any>} [root] The object containing `inside`, i.e. the object that contains the
  		 * object to be modified.
  		 *
  		 * Defaults to `Prism.languages`.
  		 * @returns {Grammar} The new grammar object.
  		 * @public
  		 */
  		insertBefore: function (inside, before, insert, root) {
  			root = root || /** @type {any} */ (_.languages);
  			var grammar = root[inside];
  			/** @type {Grammar} */
  			var ret = {};

  			for (var token in grammar) {
  				if (grammar.hasOwnProperty(token)) {

  					if (token == before) {
  						for (var newToken in insert) {
  							if (insert.hasOwnProperty(newToken)) {
  								ret[newToken] = insert[newToken];
  							}
  						}
  					}

  					// Do not insert token which also occur in insert. See #1525
  					if (!insert.hasOwnProperty(token)) {
  						ret[token] = grammar[token];
  					}
  				}
  			}

  			var old = root[inside];
  			root[inside] = ret;

  			// Update references in other language definitions
  			_.languages.DFS(_.languages, function(key, value) {
  				if (value === old && key != inside) {
  					this[key] = ret;
  				}
  			});

  			return ret;
  		},

  		// Traverse a language definition with Depth First Search
  		DFS: function DFS(o, callback, type, visited) {
  			visited = visited || {};

  			var objId = _.util.objId;

  			for (var i in o) {
  				if (o.hasOwnProperty(i)) {
  					callback.call(o, i, o[i], type || i);

  					var property = o[i],
  					    propertyType = _.util.type(property);

  					if (propertyType === 'Object' && !visited[objId(property)]) {
  						visited[objId(property)] = true;
  						DFS(property, callback, null, visited);
  					}
  					else if (propertyType === 'Array' && !visited[objId(property)]) {
  						visited[objId(property)] = true;
  						DFS(property, callback, i, visited);
  					}
  				}
  			}
  		}
  	},

  	plugins: {},

  	/**
  	 * This is the most high-level function in Prism’s API.
  	 * It fetches all the elements that have a `.language-xxxx` class and then calls {@link Prism.highlightElement} on
  	 * each one of them.
  	 *
  	 * This is equivalent to `Prism.highlightAllUnder(document, async, callback)`.
  	 *
  	 * @param {boolean} [async=false] Same as in {@link Prism.highlightAllUnder}.
  	 * @param {HighlightCallback} [callback] Same as in {@link Prism.highlightAllUnder}.
  	 * @memberof Prism
  	 * @public
  	 */
  	highlightAll: function(async, callback) {
  		_.highlightAllUnder(document, async, callback);
  	},

  	/**
  	 * Fetches all the descendants of `container` that have a `.language-xxxx` class and then calls
  	 * {@link Prism.highlightElement} on each one of them.
  	 *
  	 * The following hooks will be run:
  	 * 1. `before-highlightall`
  	 * 2. `before-all-elements-highlight`
  	 * 3. All hooks of {@link Prism.highlightElement} for each element.
  	 *
  	 * @param {ParentNode} container The root element, whose descendants that have a `.language-xxxx` class will be highlighted.
  	 * @param {boolean} [async=false] Whether each element is to be highlighted asynchronously using Web Workers.
  	 * @param {HighlightCallback} [callback] An optional callback to be invoked on each element after its highlighting is done.
  	 * @memberof Prism
  	 * @public
  	 */
  	highlightAllUnder: function(container, async, callback) {
  		var env = {
  			callback: callback,
  			container: container,
  			selector: 'code[class*="language-"], [class*="language-"] code, code[class*="lang-"], [class*="lang-"] code'
  		};

  		_.hooks.run('before-highlightall', env);

  		env.elements = Array.prototype.slice.apply(env.container.querySelectorAll(env.selector));

  		_.hooks.run('before-all-elements-highlight', env);

  		for (var i = 0, element; element = env.elements[i++];) {
  			_.highlightElement(element, async === true, env.callback);
  		}
  	},

  	/**
  	 * Highlights the code inside a single element.
  	 *
  	 * The following hooks will be run:
  	 * 1. `before-sanity-check`
  	 * 2. `before-highlight`
  	 * 3. All hooks of {@link Prism.highlight}. These hooks will be run by an asynchronous worker if `async` is `true`.
  	 * 4. `before-insert`
  	 * 5. `after-highlight`
  	 * 6. `complete`
  	 *
  	 * Some the above hooks will be skipped if the element doesn't contain any text or there is no grammar loaded for
  	 * the element's language.
  	 *
  	 * @param {Element} element The element containing the code.
  	 * It must have a class of `language-xxxx` to be processed, where `xxxx` is a valid language identifier.
  	 * @param {boolean} [async=false] Whether the element is to be highlighted asynchronously using Web Workers
  	 * to improve performance and avoid blocking the UI when highlighting very large chunks of code. This option is
  	 * [disabled by default](https://prismjs.com/faq.html#why-is-asynchronous-highlighting-disabled-by-default).
  	 *
  	 * Note: All language definitions required to highlight the code must be included in the main `prism.js` file for
  	 * asynchronous highlighting to work. You can build your own bundle on the
  	 * [Download page](https://prismjs.com/download.html).
  	 * @param {HighlightCallback} [callback] An optional callback to be invoked after the highlighting is done.
  	 * Mostly useful when `async` is `true`, since in that case, the highlighting is done asynchronously.
  	 * @memberof Prism
  	 * @public
  	 */
  	highlightElement: function(element, async, callback) {
  		// Find language
  		var language = _.util.getLanguage(element);
  		var grammar = _.languages[language];

  		// Set language on the element, if not present
  		element.className = element.className.replace(lang, '').replace(/\s+/g, ' ') + ' language-' + language;

  		// Set language on the parent, for styling
  		var parent = element.parentElement;
  		if (parent && parent.nodeName.toLowerCase() === 'pre') {
  			parent.className = parent.className.replace(lang, '').replace(/\s+/g, ' ') + ' language-' + language;
  		}

  		var code = element.textContent;

  		var env = {
  			element: element,
  			language: language,
  			grammar: grammar,
  			code: code
  		};

  		function insertHighlightedCode(highlightedCode) {
  			env.highlightedCode = highlightedCode;

  			_.hooks.run('before-insert', env);

  			env.element.innerHTML = env.highlightedCode;

  			_.hooks.run('after-highlight', env);
  			_.hooks.run('complete', env);
  			callback && callback.call(env.element);
  		}

  		_.hooks.run('before-sanity-check', env);

  		if (!env.code) {
  			_.hooks.run('complete', env);
  			callback && callback.call(env.element);
  			return;
  		}

  		_.hooks.run('before-highlight', env);

  		if (!env.grammar) {
  			insertHighlightedCode(_.util.encode(env.code));
  			return;
  		}

  		if (async && _self.Worker) {
  			var worker = new Worker(_.filename);

  			worker.onmessage = function(evt) {
  				insertHighlightedCode(evt.data);
  			};

  			worker.postMessage(JSON.stringify({
  				language: env.language,
  				code: env.code,
  				immediateClose: true
  			}));
  		}
  		else {
  			insertHighlightedCode(_.highlight(env.code, env.grammar, env.language));
  		}
  	},

  	/**
  	 * Low-level function, only use if you know what you’re doing. It accepts a string of text as input
  	 * and the language definitions to use, and returns a string with the HTML produced.
  	 *
  	 * The following hooks will be run:
  	 * 1. `before-tokenize`
  	 * 2. `after-tokenize`
  	 * 3. `wrap`: On each {@link Token}.
  	 *
  	 * @param {string} text A string with the code to be highlighted.
  	 * @param {Grammar} grammar An object containing the tokens to use.
  	 *
  	 * Usually a language definition like `Prism.languages.markup`.
  	 * @param {string} language The name of the language definition passed to `grammar`.
  	 * @returns {string} The highlighted HTML.
  	 * @memberof Prism
  	 * @public
  	 * @example
  	 * Prism.highlight('var foo = true;', Prism.languages.javascript, 'javascript');
  	 */
  	highlight: function (text, grammar, language) {
  		var env = {
  			code: text,
  			grammar: grammar,
  			language: language
  		};
  		_.hooks.run('before-tokenize', env);
  		env.tokens = _.tokenize(env.code, env.grammar);
  		_.hooks.run('after-tokenize', env);
  		return Token.stringify(_.util.encode(env.tokens), env.language);
  	},

  	/**
  	 * This is the heart of Prism, and the most low-level function you can use. It accepts a string of text as input
  	 * and the language definitions to use, and returns an array with the tokenized code.
  	 *
  	 * When the language definition includes nested tokens, the function is called recursively on each of these tokens.
  	 *
  	 * This method could be useful in other contexts as well, as a very crude parser.
  	 *
  	 * @param {string} text A string with the code to be highlighted.
  	 * @param {Grammar} grammar An object containing the tokens to use.
  	 *
  	 * Usually a language definition like `Prism.languages.markup`.
  	 * @returns {TokenStream} An array of strings and tokens, a token stream.
  	 * @memberof Prism
  	 * @public
  	 * @example
  	 * let code = `var foo = 0;`;
  	 * let tokens = Prism.tokenize(code, Prism.languages.javascript);
  	 * tokens.forEach(token => {
  	 *     if (token instanceof Prism.Token && token.type === 'number') {
  	 *         console.log(`Found numeric literal: ${token.content}`);
  	 *     }
  	 * });
  	 */
  	tokenize: function(text, grammar) {
  		var rest = grammar.rest;
  		if (rest) {
  			for (var token in rest) {
  				grammar[token] = rest[token];
  			}

  			delete grammar.rest;
  		}

  		var tokenList = new LinkedList();
  		addAfter(tokenList, tokenList.head, text);

  		matchGrammar(text, tokenList, grammar, tokenList.head, 0);

  		return toArray(tokenList);
  	},

  	/**
  	 * @namespace
  	 * @memberof Prism
  	 * @public
  	 */
  	hooks: {
  		all: {},

  		/**
  		 * Adds the given callback to the list of callbacks for the given hook.
  		 *
  		 * The callback will be invoked when the hook it is registered for is run.
  		 * Hooks are usually directly run by a highlight function but you can also run hooks yourself.
  		 *
  		 * One callback function can be registered to multiple hooks and the same hook multiple times.
  		 *
  		 * @param {string} name The name of the hook.
  		 * @param {HookCallback} callback The callback function which is given environment variables.
  		 * @public
  		 */
  		add: function (name, callback) {
  			var hooks = _.hooks.all;

  			hooks[name] = hooks[name] || [];

  			hooks[name].push(callback);
  		},

  		/**
  		 * Runs a hook invoking all registered callbacks with the given environment variables.
  		 *
  		 * Callbacks will be invoked synchronously and in the order in which they were registered.
  		 *
  		 * @param {string} name The name of the hook.
  		 * @param {Object<string, any>} env The environment variables of the hook passed to all callbacks registered.
  		 * @public
  		 */
  		run: function (name, env) {
  			var callbacks = _.hooks.all[name];

  			if (!callbacks || !callbacks.length) {
  				return;
  			}

  			for (var i=0, callback; callback = callbacks[i++];) {
  				callback(env);
  			}
  		}
  	},

  	Token: Token
  };
  _self.Prism = _;


  // Typescript note:
  // The following can be used to import the Token type in JSDoc:
  //
  //   @typedef {InstanceType<import("./prism-core")["Token"]>} Token

  /**
   * Creates a new token.
   *
   * @param {string} type See {@link Token#type type}
   * @param {string | TokenStream} content See {@link Token#content content}
   * @param {string|string[]} [alias] The alias(es) of the token.
   * @param {string} [matchedStr=""] A copy of the full string this token was created from.
   * @class
   * @global
   * @public
   */
  function Token(type, content, alias, matchedStr) {
  	/**
  	 * The type of the token.
  	 *
  	 * This is usually the key of a pattern in a {@link Grammar}.
  	 *
  	 * @type {string}
  	 * @see GrammarToken
  	 * @public
  	 */
  	this.type = type;
  	/**
  	 * The strings or tokens contained by this token.
  	 *
  	 * This will be a token stream if the pattern matched also defined an `inside` grammar.
  	 *
  	 * @type {string | TokenStream}
  	 * @public
  	 */
  	this.content = content;
  	/**
  	 * The alias(es) of the token.
  	 *
  	 * @type {string|string[]}
  	 * @see GrammarToken
  	 * @public
  	 */
  	this.alias = alias;
  	// Copy of the full string this token was created from
  	this.length = (matchedStr || '').length | 0;
  }

  /**
   * A token stream is an array of strings and {@link Token Token} objects.
   *
   * Token streams have to fulfill a few properties that are assumed by most functions (mostly internal ones) that process
   * them.
   *
   * 1. No adjacent strings.
   * 2. No empty strings.
   *
   *    The only exception here is the token stream that only contains the empty string and nothing else.
   *
   * @typedef {Array<string | Token>} TokenStream
   * @global
   * @public
   */

  /**
   * Converts the given token or token stream to an HTML representation.
   *
   * The following hooks will be run:
   * 1. `wrap`: On each {@link Token}.
   *
   * @param {string | Token | TokenStream} o The token or token stream to be converted.
   * @param {string} language The name of current language.
   * @returns {string} The HTML representation of the token or token stream.
   * @memberof Token
   * @static
   */
  Token.stringify = function stringify(o, language) {
  	if (typeof o == 'string') {
  		return o;
  	}
  	if (Array.isArray(o)) {
  		var s = '';
  		o.forEach(function (e) {
  			s += stringify(e, language);
  		});
  		return s;
  	}

  	var env = {
  		type: o.type,
  		content: stringify(o.content, language),
  		tag: 'span',
  		classes: ['token', o.type],
  		attributes: {},
  		language: language
  	};

  	var aliases = o.alias;
  	if (aliases) {
  		if (Array.isArray(aliases)) {
  			Array.prototype.push.apply(env.classes, aliases);
  		} else {
  			env.classes.push(aliases);
  		}
  	}

  	_.hooks.run('wrap', env);

  	var attributes = '';
  	for (var name in env.attributes) {
  		attributes += ' ' + name + '="' + (env.attributes[name] || '').replace(/"/g, '&quot;') + '"';
  	}

  	return '<' + env.tag + ' class="' + env.classes.join(' ') + '"' + attributes + '>' + env.content + '</' + env.tag + '>';
  };

  /**
   * @param {RegExp} pattern
   * @param {number} pos
   * @param {string} text
   * @param {boolean} lookbehind
   * @returns {RegExpExecArray | null}
   */
  function matchPattern(pattern, pos, text, lookbehind) {
  	pattern.lastIndex = pos;
  	var match = pattern.exec(text);
  	if (match && lookbehind && match[1]) {
  		// change the match to remove the text matched by the Prism lookbehind group
  		var lookbehindLength = match[1].length;
  		match.index += lookbehindLength;
  		match[0] = match[0].slice(lookbehindLength);
  	}
  	return match;
  }

  /**
   * @param {string} text
   * @param {LinkedList<string | Token>} tokenList
   * @param {any} grammar
   * @param {LinkedListNode<string | Token>} startNode
   * @param {number} startPos
   * @param {RematchOptions} [rematch]
   * @returns {void}
   * @private
   *
   * @typedef RematchOptions
   * @property {string} cause
   * @property {number} reach
   */
  function matchGrammar(text, tokenList, grammar, startNode, startPos, rematch) {
  	for (var token in grammar) {
  		if (!grammar.hasOwnProperty(token) || !grammar[token]) {
  			continue;
  		}

  		var patterns = grammar[token];
  		patterns = Array.isArray(patterns) ? patterns : [patterns];

  		for (var j = 0; j < patterns.length; ++j) {
  			if (rematch && rematch.cause == token + ',' + j) {
  				return;
  			}

  			var patternObj = patterns[j],
  				inside = patternObj.inside,
  				lookbehind = !!patternObj.lookbehind,
  				greedy = !!patternObj.greedy,
  				alias = patternObj.alias;

  			if (greedy && !patternObj.pattern.global) {
  				// Without the global flag, lastIndex won't work
  				var flags = patternObj.pattern.toString().match(/[imsuy]*$/)[0];
  				patternObj.pattern = RegExp(patternObj.pattern.source, flags + 'g');
  			}

  			/** @type {RegExp} */
  			var pattern = patternObj.pattern || patternObj;

  			for ( // iterate the token list and keep track of the current token/string position
  				var currentNode = startNode.next, pos = startPos;
  				currentNode !== tokenList.tail;
  				pos += currentNode.value.length, currentNode = currentNode.next
  			) {

  				if (rematch && pos >= rematch.reach) {
  					break;
  				}

  				var str = currentNode.value;

  				if (tokenList.length > text.length) {
  					// Something went terribly wrong, ABORT, ABORT!
  					return;
  				}

  				if (str instanceof Token) {
  					continue;
  				}

  				var removeCount = 1; // this is the to parameter of removeBetween
  				var match;

  				if (greedy) {
  					match = matchPattern(pattern, pos, text, lookbehind);
  					if (!match) {
  						break;
  					}

  					var from = match.index;
  					var to = match.index + match[0].length;
  					var p = pos;

  					// find the node that contains the match
  					p += currentNode.value.length;
  					while (from >= p) {
  						currentNode = currentNode.next;
  						p += currentNode.value.length;
  					}
  					// adjust pos (and p)
  					p -= currentNode.value.length;
  					pos = p;

  					// the current node is a Token, then the match starts inside another Token, which is invalid
  					if (currentNode.value instanceof Token) {
  						continue;
  					}

  					// find the last node which is affected by this match
  					for (
  						var k = currentNode;
  						k !== tokenList.tail && (p < to || typeof k.value === 'string');
  						k = k.next
  					) {
  						removeCount++;
  						p += k.value.length;
  					}
  					removeCount--;

  					// replace with the new match
  					str = text.slice(pos, p);
  					match.index -= pos;
  				} else {
  					match = matchPattern(pattern, 0, str, lookbehind);
  					if (!match) {
  						continue;
  					}
  				}

  				var from = match.index,
  					matchStr = match[0],
  					before = str.slice(0, from),
  					after = str.slice(from + matchStr.length);

  				var reach = pos + str.length;
  				if (rematch && reach > rematch.reach) {
  					rematch.reach = reach;
  				}

  				var removeFrom = currentNode.prev;

  				if (before) {
  					removeFrom = addAfter(tokenList, removeFrom, before);
  					pos += before.length;
  				}

  				removeRange(tokenList, removeFrom, removeCount);

  				var wrapped = new Token(token, inside ? _.tokenize(matchStr, inside) : matchStr, alias, matchStr);
  				currentNode = addAfter(tokenList, removeFrom, wrapped);

  				if (after) {
  					addAfter(tokenList, currentNode, after);
  				}

  				if (removeCount > 1) {
  					// at least one Token object was removed, so we have to do some rematching
  					// this can only happen if the current pattern is greedy
  					matchGrammar(text, tokenList, grammar, currentNode.prev, pos, {
  						cause: token + ',' + j,
  						reach: reach
  					});
  				}
  			}
  		}
  	}
  }

  /**
   * @typedef LinkedListNode
   * @property {T} value
   * @property {LinkedListNode<T> | null} prev The previous node.
   * @property {LinkedListNode<T> | null} next The next node.
   * @template T
   * @private
   */

  /**
   * @template T
   * @private
   */
  function LinkedList() {
  	/** @type {LinkedListNode<T>} */
  	var head = { value: null, prev: null, next: null };
  	/** @type {LinkedListNode<T>} */
  	var tail = { value: null, prev: head, next: null };
  	head.next = tail;

  	/** @type {LinkedListNode<T>} */
  	this.head = head;
  	/** @type {LinkedListNode<T>} */
  	this.tail = tail;
  	this.length = 0;
  }

  /**
   * Adds a new node with the given value to the list.
   * @param {LinkedList<T>} list
   * @param {LinkedListNode<T>} node
   * @param {T} value
   * @returns {LinkedListNode<T>} The added node.
   * @template T
   */
  function addAfter(list, node, value) {
  	// assumes that node != list.tail && values.length >= 0
  	var next = node.next;

  	var newNode = { value: value, prev: node, next: next };
  	node.next = newNode;
  	next.prev = newNode;
  	list.length++;

  	return newNode;
  }
  /**
   * Removes `count` nodes after the given node. The given node will not be removed.
   * @param {LinkedList<T>} list
   * @param {LinkedListNode<T>} node
   * @param {number} count
   * @template T
   */
  function removeRange(list, node, count) {
  	var next = node.next;
  	for (var i = 0; i < count && next !== list.tail; i++) {
  		next = next.next;
  	}
  	node.next = next;
  	next.prev = node;
  	list.length -= i;
  }
  /**
   * @param {LinkedList<T>} list
   * @returns {T[]}
   * @template T
   */
  function toArray(list) {
  	var array = [];
  	var node = list.head.next;
  	while (node !== list.tail) {
  		array.push(node.value);
  		node = node.next;
  	}
  	return array;
  }


  if (!_self.document) {
  	if (!_self.addEventListener) {
  		// in Node.js
  		return _;
  	}

  	if (!_.disableWorkerMessageHandler) {
  		// In worker
  		_self.addEventListener('message', function (evt) {
  			var message = JSON.parse(evt.data),
  				lang = message.language,
  				code = message.code,
  				immediateClose = message.immediateClose;

  			_self.postMessage(_.highlight(code, _.languages[lang], lang));
  			if (immediateClose) {
  				_self.close();
  			}
  		}, false);
  	}

  	return _;
  }

  // Get current script and highlight
  var script = _.util.currentScript();

  if (script) {
  	_.filename = script.src;

  	if (script.hasAttribute('data-manual')) {
  		_.manual = true;
  	}
  }

  function highlightAutomaticallyCallback() {
  	if (!_.manual) {
  		_.highlightAll();
  	}
  }

  if (!_.manual) {
  	// If the document state is "loading", then we'll use DOMContentLoaded.
  	// If the document state is "interactive" and the prism.js script is deferred, then we'll also use the
  	// DOMContentLoaded event because there might be some plugins or languages which have also been deferred and they
  	// might take longer one animation frame to execute which can create a race condition where only some plugins have
  	// been loaded when Prism.highlightAll() is executed, depending on how fast resources are loaded.
  	// See https://github.com/PrismJS/prism/issues/2102
  	var readyState = document.readyState;
  	if (readyState === 'loading' || readyState === 'interactive' && script && script.defer) {
  		document.addEventListener('DOMContentLoaded', highlightAutomaticallyCallback);
  	} else {
  		if (window.requestAnimationFrame) {
  			window.requestAnimationFrame(highlightAutomaticallyCallback);
  		} else {
  			window.setTimeout(highlightAutomaticallyCallback, 16);
  		}
  	}
  }

  return _;

  })(_self);

  if ( module.exports) {
  	module.exports = Prism;
  }

  // hack for components to work correctly in node.js
  if (typeof commonjsGlobal !== 'undefined') {
  	commonjsGlobal.Prism = Prism;
  }

  // some additional documentation/types

  /**
   * The expansion of a simple `RegExp` literal to support additional properties.
   *
   * @typedef GrammarToken
   * @property {RegExp} pattern The regular expression of the token.
   * @property {boolean} [lookbehind=false] If `true`, then the first capturing group of `pattern` will (effectively)
   * behave as a lookbehind group meaning that the captured text will not be part of the matched text of the new token.
   * @property {boolean} [greedy=false] Whether the token is greedy.
   * @property {string|string[]} [alias] An optional alias or list of aliases.
   * @property {Grammar} [inside] The nested grammar of this token.
   *
   * The `inside` grammar will be used to tokenize the text value of each token of this kind.
   *
   * This can be used to make nested and even recursive language definitions.
   *
   * Note: This can cause infinite recursion. Be careful when you embed different languages or even the same language into
   * each another.
   * @global
   * @public
  */

  /**
   * @typedef Grammar
   * @type {Object<string, RegExp | GrammarToken | Array<RegExp | GrammarToken>>}
   * @property {Grammar} [rest] An optional grammar object that will be appended to this grammar.
   * @global
   * @public
   */

  /**
   * A function which will invoked after an element was successfully highlighted.
   *
   * @callback HighlightCallback
   * @param {Element} element The element successfully highlighted.
   * @returns {void}
   * @global
   * @public
  */

  /**
   * @callback HookCallback
   * @param {Object<string, any>} env The environment variables of the hook.
   * @returns {void}
   * @global
   * @public
   */


  /* **********************************************
       Begin prism-markup.js
  ********************************************** */

  Prism.languages.markup = {
  	'comment': /<!--[\s\S]*?-->/,
  	'prolog': /<\?[\s\S]+?\?>/,
  	'doctype': {
  		// https://www.w3.org/TR/xml/#NT-doctypedecl
  		pattern: /<!DOCTYPE(?:[^>"'[\]]|"[^"]*"|'[^']*')+(?:\[(?:[^<"'\]]|"[^"]*"|'[^']*'|<(?!!--)|<!--(?:[^-]|-(?!->))*-->)*\]\s*)?>/i,
  		greedy: true,
  		inside: {
  			'internal-subset': {
  				pattern: /(\[)[\s\S]+(?=\]>$)/,
  				lookbehind: true,
  				greedy: true,
  				inside: null // see below
  			},
  			'string': {
  				pattern: /"[^"]*"|'[^']*'/,
  				greedy: true
  			},
  			'punctuation': /^<!|>$|[[\]]/,
  			'doctype-tag': /^DOCTYPE/,
  			'name': /[^\s<>'"]+/
  		}
  	},
  	'cdata': /<!\[CDATA\[[\s\S]*?]]>/i,
  	'tag': {
  		pattern: /<\/?(?!\d)[^\s>\/=$<%]+(?:\s(?:\s*[^\s>\/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+(?=[\s>]))|(?=[\s/>])))+)?\s*\/?>/,
  		greedy: true,
  		inside: {
  			'tag': {
  				pattern: /^<\/?[^\s>\/]+/,
  				inside: {
  					'punctuation': /^<\/?/,
  					'namespace': /^[^\s>\/:]+:/
  				}
  			},
  			'attr-value': {
  				pattern: /=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+)/,
  				inside: {
  					'punctuation': [
  						{
  							pattern: /^=/,
  							alias: 'attr-equals'
  						},
  						/"|'/
  					]
  				}
  			},
  			'punctuation': /\/?>/,
  			'attr-name': {
  				pattern: /[^\s>\/]+/,
  				inside: {
  					'namespace': /^[^\s>\/:]+:/
  				}
  			}

  		}
  	},
  	'entity': [
  		{
  			pattern: /&[\da-z]{1,8};/i,
  			alias: 'named-entity'
  		},
  		/&#x?[\da-f]{1,8};/i
  	]
  };

  Prism.languages.markup['tag'].inside['attr-value'].inside['entity'] =
  	Prism.languages.markup['entity'];
  Prism.languages.markup['doctype'].inside['internal-subset'].inside = Prism.languages.markup;

  // Plugin to make entity title show the real entity, idea by Roman Komarov
  Prism.hooks.add('wrap', function (env) {

  	if (env.type === 'entity') {
  		env.attributes['title'] = env.content.replace(/&amp;/, '&');
  	}
  });

  Object.defineProperty(Prism.languages.markup.tag, 'addInlined', {
  	/**
  	 * Adds an inlined language to markup.
  	 *
  	 * An example of an inlined language is CSS with `<style>` tags.
  	 *
  	 * @param {string} tagName The name of the tag that contains the inlined language. This name will be treated as
  	 * case insensitive.
  	 * @param {string} lang The language key.
  	 * @example
  	 * addInlined('style', 'css');
  	 */
  	value: function addInlined(tagName, lang) {
  		var includedCdataInside = {};
  		includedCdataInside['language-' + lang] = {
  			pattern: /(^<!\[CDATA\[)[\s\S]+?(?=\]\]>$)/i,
  			lookbehind: true,
  			inside: Prism.languages[lang]
  		};
  		includedCdataInside['cdata'] = /^<!\[CDATA\[|\]\]>$/i;

  		var inside = {
  			'included-cdata': {
  				pattern: /<!\[CDATA\[[\s\S]*?\]\]>/i,
  				inside: includedCdataInside
  			}
  		};
  		inside['language-' + lang] = {
  			pattern: /[\s\S]+/,
  			inside: Prism.languages[lang]
  		};

  		var def = {};
  		def[tagName] = {
  			pattern: RegExp(/(<__[^>]*>)(?:<!\[CDATA\[(?:[^\]]|\](?!\]>))*\]\]>|(?!<!\[CDATA\[)[\s\S])*?(?=<\/__>)/.source.replace(/__/g, function () { return tagName; }), 'i'),
  			lookbehind: true,
  			greedy: true,
  			inside: inside
  		};

  		Prism.languages.insertBefore('markup', 'cdata', def);
  	}
  });

  Prism.languages.html = Prism.languages.markup;
  Prism.languages.mathml = Prism.languages.markup;
  Prism.languages.svg = Prism.languages.markup;

  Prism.languages.xml = Prism.languages.extend('markup', {});
  Prism.languages.ssml = Prism.languages.xml;
  Prism.languages.atom = Prism.languages.xml;
  Prism.languages.rss = Prism.languages.xml;


  /* **********************************************
       Begin prism-css.js
  ********************************************** */

  (function (Prism) {

  	var string = /("|')(?:\\(?:\r\n|[\s\S])|(?!\1)[^\\\r\n])*\1/;

  	Prism.languages.css = {
  		'comment': /\/\*[\s\S]*?\*\//,
  		'atrule': {
  			pattern: /@[\w-](?:[^;{\s]|\s+(?![\s{]))*(?:;|(?=\s*\{))/,
  			inside: {
  				'rule': /^@[\w-]+/,
  				'selector-function-argument': {
  					pattern: /(\bselector\s*\(\s*(?![\s)]))(?:[^()\s]|\s+(?![\s)])|\((?:[^()]|\([^()]*\))*\))+(?=\s*\))/,
  					lookbehind: true,
  					alias: 'selector'
  				},
  				'keyword': {
  					pattern: /(^|[^\w-])(?:and|not|only|or)(?![\w-])/,
  					lookbehind: true
  				}
  				// See rest below
  			}
  		},
  		'url': {
  			// https://drafts.csswg.org/css-values-3/#urls
  			pattern: RegExp('\\burl\\((?:' + string.source + '|' + /(?:[^\\\r\n()"']|\\[\s\S])*/.source + ')\\)', 'i'),
  			greedy: true,
  			inside: {
  				'function': /^url/i,
  				'punctuation': /^\(|\)$/,
  				'string': {
  					pattern: RegExp('^' + string.source + '$'),
  					alias: 'url'
  				}
  			}
  		},
  		'selector': RegExp('[^{}\\s](?:[^{};"\'\\s]|\\s+(?![\\s{])|' + string.source + ')*(?=\\s*\\{)'),
  		'string': {
  			pattern: string,
  			greedy: true
  		},
  		'property': /(?!\s)[-_a-z\xA0-\uFFFF](?:(?!\s)[-\w\xA0-\uFFFF])*(?=\s*:)/i,
  		'important': /!important\b/i,
  		'function': /[-a-z0-9]+(?=\()/i,
  		'punctuation': /[(){};:,]/
  	};

  	Prism.languages.css['atrule'].inside.rest = Prism.languages.css;

  	var markup = Prism.languages.markup;
  	if (markup) {
  		markup.tag.addInlined('style', 'css');

  		Prism.languages.insertBefore('inside', 'attr-value', {
  			'style-attr': {
  				pattern: /(^|["'\s])style\s*=\s*(?:"[^"]*"|'[^']*')/i,
  				lookbehind: true,
  				inside: {
  					'attr-value': {
  						pattern: /=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+)/,
  						inside: {
  							'style': {
  								pattern: /(["'])[\s\S]+(?=["']$)/,
  								lookbehind: true,
  								alias: 'language-css',
  								inside: Prism.languages.css
  							},
  							'punctuation': [
  								{
  									pattern: /^=/,
  									alias: 'attr-equals'
  								},
  								/"|'/
  							]
  						}
  					},
  					'attr-name': /^style/i
  				}
  			}
  		}, markup.tag);
  	}

  }(Prism));


  /* **********************************************
       Begin prism-clike.js
  ********************************************** */

  Prism.languages.clike = {
  	'comment': [
  		{
  			pattern: /(^|[^\\])\/\*[\s\S]*?(?:\*\/|$)/,
  			lookbehind: true,
  			greedy: true
  		},
  		{
  			pattern: /(^|[^\\:])\/\/.*/,
  			lookbehind: true,
  			greedy: true
  		}
  	],
  	'string': {
  		pattern: /(["'])(?:\\(?:\r\n|[\s\S])|(?!\1)[^\\\r\n])*\1/,
  		greedy: true
  	},
  	'class-name': {
  		pattern: /(\b(?:class|interface|extends|implements|trait|instanceof|new)\s+|\bcatch\s+\()[\w.\\]+/i,
  		lookbehind: true,
  		inside: {
  			'punctuation': /[.\\]/
  		}
  	},
  	'keyword': /\b(?:if|else|while|do|for|return|in|instanceof|function|new|try|throw|catch|finally|null|break|continue)\b/,
  	'boolean': /\b(?:true|false)\b/,
  	'function': /\w+(?=\()/,
  	'number': /\b0x[\da-f]+\b|(?:\b\d+(?:\.\d*)?|\B\.\d+)(?:e[+-]?\d+)?/i,
  	'operator': /[<>]=?|[!=]=?=?|--?|\+\+?|&&?|\|\|?|[?*/~^%]/,
  	'punctuation': /[{}[\];(),.:]/
  };


  /* **********************************************
       Begin prism-javascript.js
  ********************************************** */

  Prism.languages.javascript = Prism.languages.extend('clike', {
  	'class-name': [
  		Prism.languages.clike['class-name'],
  		{
  			pattern: /(^|[^$\w\xA0-\uFFFF])(?!\s)[_$A-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\.(?:prototype|constructor))/,
  			lookbehind: true
  		}
  	],
  	'keyword': [
  		{
  			pattern: /((?:^|})\s*)(?:catch|finally)\b/,
  			lookbehind: true
  		},
  		{
  			pattern: /(^|[^.]|\.\.\.\s*)\b(?:as|async(?=\s*(?:function\b|\(|[$\w\xA0-\uFFFF]|$))|await|break|case|class|const|continue|debugger|default|delete|do|else|enum|export|extends|for|from|function|(?:get|set)(?=\s*[\[$\w\xA0-\uFFFF])|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)\b/,
  			lookbehind: true
  		} ],
  	// Allow for all non-ASCII characters (See http://stackoverflow.com/a/2008444)
  	'function': /#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*(?:\.\s*(?:apply|bind|call)\s*)?\()/,
  	'number': /\b(?:(?:0[xX](?:[\dA-Fa-f](?:_[\dA-Fa-f])?)+|0[bB](?:[01](?:_[01])?)+|0[oO](?:[0-7](?:_[0-7])?)+)n?|(?:\d(?:_\d)?)+n|NaN|Infinity)\b|(?:\b(?:\d(?:_\d)?)+\.?(?:\d(?:_\d)?)*|\B\.(?:\d(?:_\d)?)+)(?:[Ee][+-]?(?:\d(?:_\d)?)+)?/,
  	'operator': /--|\+\+|\*\*=?|=>|&&=?|\|\|=?|[!=]==|<<=?|>>>?=?|[-+*/%&|^!=<>]=?|\.{3}|\?\?=?|\?\.?|[~:]/
  });

  Prism.languages.javascript['class-name'][0].pattern = /(\b(?:class|interface|extends|implements|instanceof|new)\s+)[\w.\\]+/;

  Prism.languages.insertBefore('javascript', 'keyword', {
  	'regex': {
  		pattern: /((?:^|[^$\w\xA0-\uFFFF."'\])\s]|\b(?:return|yield))\s*)\/(?:\[(?:[^\]\\\r\n]|\\.)*]|\\.|[^/\\\[\r\n])+\/[gimyus]{0,6}(?=(?:\s|\/\*(?:[^*]|\*(?!\/))*\*\/)*(?:$|[\r\n,.;:})\]]|\/\/))/,
  		lookbehind: true,
  		greedy: true,
  		inside: {
  			'regex-source': {
  				pattern: /^(\/)[\s\S]+(?=\/[a-z]*$)/,
  				lookbehind: true,
  				alias: 'language-regex',
  				inside: Prism.languages.regex
  			},
  			'regex-flags': /[a-z]+$/,
  			'regex-delimiter': /^\/|\/$/
  		}
  	},
  	// This must be declared before keyword because we use "function" inside the look-forward
  	'function-variable': {
  		pattern: /#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*[=:]\s*(?:async\s*)?(?:\bfunction\b|(?:\((?:[^()]|\([^()]*\))*\)|(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)\s*=>))/,
  		alias: 'function'
  	},
  	'parameter': [
  		{
  			pattern: /(function(?:\s+(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)?\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\))/,
  			lookbehind: true,
  			inside: Prism.languages.javascript
  		},
  		{
  			pattern: /(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*=>)/i,
  			inside: Prism.languages.javascript
  		},
  		{
  			pattern: /(\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*=>)/,
  			lookbehind: true,
  			inside: Prism.languages.javascript
  		},
  		{
  			pattern: /((?:\b|\s|^)(?!(?:as|async|await|break|case|catch|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally|for|from|function|get|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|set|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)(?![$\w\xA0-\uFFFF]))(?:(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*\s*)\(\s*|\]\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*\{)/,
  			lookbehind: true,
  			inside: Prism.languages.javascript
  		}
  	],
  	'constant': /\b[A-Z](?:[A-Z_]|\dx?)*\b/
  });

  Prism.languages.insertBefore('javascript', 'string', {
  	'template-string': {
  		pattern: /`(?:\\[\s\S]|\${(?:[^{}]|{(?:[^{}]|{[^}]*})*})+}|(?!\${)[^\\`])*`/,
  		greedy: true,
  		inside: {
  			'template-punctuation': {
  				pattern: /^`|`$/,
  				alias: 'string'
  			},
  			'interpolation': {
  				pattern: /((?:^|[^\\])(?:\\{2})*)\${(?:[^{}]|{(?:[^{}]|{[^}]*})*})+}/,
  				lookbehind: true,
  				inside: {
  					'interpolation-punctuation': {
  						pattern: /^\${|}$/,
  						alias: 'punctuation'
  					},
  					rest: Prism.languages.javascript
  				}
  			},
  			'string': /[\s\S]+/
  		}
  	}
  });

  if (Prism.languages.markup) {
  	Prism.languages.markup.tag.addInlined('script', 'javascript');
  }

  Prism.languages.js = Prism.languages.javascript;


  /* **********************************************
       Begin prism-file-highlight.js
  ********************************************** */

  (function () {
  	if (typeof self === 'undefined' || !self.Prism || !self.document) {
  		return;
  	}

  	// https://developer.mozilla.org/en-US/docs/Web/API/Element/matches#Polyfill
  	if (!Element.prototype.matches) {
  		Element.prototype.matches = Element.prototype.msMatchesSelector || Element.prototype.webkitMatchesSelector;
  	}

  	var Prism = window.Prism;

  	var LOADING_MESSAGE = 'Loading…';
  	var FAILURE_MESSAGE = function (status, message) {
  		return '✖ Error ' + status + ' while fetching file: ' + message;
  	};
  	var FAILURE_EMPTY_MESSAGE = '✖ Error: File does not exist or is empty';

  	var EXTENSIONS = {
  		'js': 'javascript',
  		'py': 'python',
  		'rb': 'ruby',
  		'ps1': 'powershell',
  		'psm1': 'powershell',
  		'sh': 'bash',
  		'bat': 'batch',
  		'h': 'c',
  		'tex': 'latex'
  	};

  	var STATUS_ATTR = 'data-src-status';
  	var STATUS_LOADING = 'loading';
  	var STATUS_LOADED = 'loaded';
  	var STATUS_FAILED = 'failed';

  	var SELECTOR = 'pre[data-src]:not([' + STATUS_ATTR + '="' + STATUS_LOADED + '"])'
  		+ ':not([' + STATUS_ATTR + '="' + STATUS_LOADING + '"])';

  	var lang = /\blang(?:uage)?-([\w-]+)\b/i;

  	/**
  	 * Sets the Prism `language-xxxx` or `lang-xxxx` class to the given language.
  	 *
  	 * @param {HTMLElement} element
  	 * @param {string} language
  	 * @returns {void}
  	 */
  	function setLanguageClass(element, language) {
  		var className = element.className;
  		className = className.replace(lang, ' ') + ' language-' + language;
  		element.className = className.replace(/\s+/g, ' ').trim();
  	}


  	Prism.hooks.add('before-highlightall', function (env) {
  		env.selector += ', ' + SELECTOR;
  	});

  	Prism.hooks.add('before-sanity-check', function (env) {
  		var pre = /** @type {HTMLPreElement} */ (env.element);
  		if (pre.matches(SELECTOR)) {
  			env.code = ''; // fast-path the whole thing and go to complete

  			pre.setAttribute(STATUS_ATTR, STATUS_LOADING); // mark as loading

  			// add code element with loading message
  			var code = pre.appendChild(document.createElement('CODE'));
  			code.textContent = LOADING_MESSAGE;

  			var src = pre.getAttribute('data-src');

  			var language = env.language;
  			if (language === 'none') {
  				// the language might be 'none' because there is no language set;
  				// in this case, we want to use the extension as the language
  				var extension = (/\.(\w+)$/.exec(src) || [, 'none'])[1];
  				language = EXTENSIONS[extension] || extension;
  			}

  			// set language classes
  			setLanguageClass(code, language);
  			setLanguageClass(pre, language);

  			// preload the language
  			var autoloader = Prism.plugins.autoloader;
  			if (autoloader) {
  				autoloader.loadLanguages(language);
  			}

  			// load file
  			var xhr = new XMLHttpRequest();
  			xhr.open('GET', src, true);
  			xhr.onreadystatechange = function () {
  				if (xhr.readyState == 4) {
  					if (xhr.status < 400 && xhr.responseText) {
  						// mark as loaded
  						pre.setAttribute(STATUS_ATTR, STATUS_LOADED);

  						// highlight code
  						code.textContent = xhr.responseText;
  						Prism.highlightElement(code);

  					} else {
  						// mark as failed
  						pre.setAttribute(STATUS_ATTR, STATUS_FAILED);

  						if (xhr.status >= 400) {
  							code.textContent = FAILURE_MESSAGE(xhr.status, xhr.statusText);
  						} else {
  							code.textContent = FAILURE_EMPTY_MESSAGE;
  						}
  					}
  				}
  			};
  			xhr.send(null);
  		}
  	});

  	Prism.plugins.fileHighlight = {
  		/**
  		 * Executes the File Highlight plugin for all matching `pre` elements under the given container.
  		 *
  		 * Note: Elements which are already loaded or currently loading will not be touched by this method.
  		 *
  		 * @param {ParentNode} [container=document]
  		 */
  		highlight: function highlight(container) {
  			var elements = (container || document).querySelectorAll(SELECTOR);

  			for (var i = 0, element; element = elements[i++];) {
  				Prism.highlightElement(element);
  			}
  		}
  	};

  	var logged = false;
  	/** @deprecated Use `Prism.plugins.fileHighlight.highlight` instead. */
  	Prism.fileHighlight = function () {
  		if (!logged) {
  			console.warn('Prism.fileHighlight is deprecated. Use `Prism.plugins.fileHighlight.highlight` instead.');
  			logged = true;
  		}
  		Prism.plugins.fileHighlight.highlight.apply(this, arguments);
  	};

  })();
  });

  (function (Prism) {

  	/**
  	 * Returns the placeholder for the given language id and index.
  	 *
  	 * @param {string} language
  	 * @param {string|number} index
  	 * @returns {string}
  	 */
  	function getPlaceholder(language, index) {
  		return '___' + language.toUpperCase() + index + '___';
  	}

  	Object.defineProperties(Prism.languages['markup-templating'] = {}, {
  		buildPlaceholders: {
  			/**
  			 * Tokenize all inline templating expressions matching `placeholderPattern`.
  			 *
  			 * If `replaceFilter` is provided, only matches of `placeholderPattern` for which `replaceFilter` returns
  			 * `true` will be replaced.
  			 *
  			 * @param {object} env The environment of the `before-tokenize` hook.
  			 * @param {string} language The language id.
  			 * @param {RegExp} placeholderPattern The matches of this pattern will be replaced by placeholders.
  			 * @param {(match: string) => boolean} [replaceFilter]
  			 */
  			value: function (env, language, placeholderPattern, replaceFilter) {
  				if (env.language !== language) {
  					return;
  				}

  				var tokenStack = env.tokenStack = [];

  				env.code = env.code.replace(placeholderPattern, function (match) {
  					if (typeof replaceFilter === 'function' && !replaceFilter(match)) {
  						return match;
  					}
  					var i = tokenStack.length;
  					var placeholder;

  					// Check for existing strings
  					while (env.code.indexOf(placeholder = getPlaceholder(language, i)) !== -1)
  						{ ++i; }

  					// Create a sparse array
  					tokenStack[i] = match;

  					return placeholder;
  				});

  				// Switch the grammar to markup
  				env.grammar = Prism.languages.markup;
  			}
  		},
  		tokenizePlaceholders: {
  			/**
  			 * Replace placeholders with proper tokens after tokenizing.
  			 *
  			 * @param {object} env The environment of the `after-tokenize` hook.
  			 * @param {string} language The language id.
  			 */
  			value: function (env, language) {
  				if (env.language !== language || !env.tokenStack) {
  					return;
  				}

  				// Switch the grammar back
  				env.grammar = Prism.languages[language];

  				var j = 0;
  				var keys = Object.keys(env.tokenStack);

  				function walkTokens(tokens) {
  					for (var i = 0; i < tokens.length; i++) {
  						// all placeholders are replaced already
  						if (j >= keys.length) {
  							break;
  						}

  						var token = tokens[i];
  						if (typeof token === 'string' || (token.content && typeof token.content === 'string')) {
  							var k = keys[j];
  							var t = env.tokenStack[k];
  							var s = typeof token === 'string' ? token : token.content;
  							var placeholder = getPlaceholder(language, k);

  							var index = s.indexOf(placeholder);
  							if (index > -1) {
  								++j;

  								var before = s.substring(0, index);
  								var middle = new Prism.Token(language, Prism.tokenize(t, env.grammar), 'language-' + language, t);
  								var after = s.substring(index + placeholder.length);

  								var replacement = [];
  								if (before) {
  									replacement.push.apply(replacement, walkTokens([before]));
  								}
  								replacement.push(middle);
  								if (after) {
  									replacement.push.apply(replacement, walkTokens([after]));
  								}

  								if (typeof token === 'string') {
  									tokens.splice.apply(tokens, [i, 1].concat(replacement));
  								} else {
  									token.content = replacement;
  								}
  							}
  						} else if (token.content /* && typeof token.content !== 'string' */) {
  							walkTokens(token.content);
  						}
  					}

  					return tokens;
  				}

  				walkTokens(env.tokens);
  			}
  		}
  	});

  }(Prism));

  var highlightCodeCompiler = function (ref) {
      var renderer = ref.renderer;

      return (renderer.code = function(code, lang) {
      if ( lang === void 0 ) lang = 'markup';

      var langOrMarkup = prism.languages[lang] || prism.languages.markup;
      var text = prism.highlight(
        code.replace(/@DOCSIFY_QM@/g, '`'),
        langOrMarkup,
        lang
      );

      return ("<pre v-pre data-lang=\"" + lang + "\"><code class=\"lang-" + lang + "\">" + text + "</code></pre>");
    });
  };

  var paragraphCompiler = function (ref) {
      var renderer = ref.renderer;

      return (renderer.paragraph = function (text) {
      var result;
      if (/^!&gt;/.test(text)) {
        result = helper('tip', text);
      } else if (/^\?&gt;/.test(text)) {
        result = helper('warn', text);
      } else {
        result = "<p>" + text + "</p>";
      }

      return result;
    });
  };

  var taskListCompiler = function (ref) {
      var renderer = ref.renderer;

      return (renderer.list = function (body, ordered, start) {
      var isTaskList = /<li class="task-list-item">/.test(
        body.split('class="task-list"')[0]
      );
      var isStartReq = start && start > 1;
      var tag = ordered ? 'ol' : 'ul';
      var tagAttrs = [
        isTaskList ? 'class="task-list"' : '',
        isStartReq ? ("start=\"" + start + "\"") : '' ]
        .join(' ')
        .trim();

      return ("<" + tag + " " + tagAttrs + ">" + body + "</" + tag + ">");
    });
  };

  var taskListItemCompiler = function (ref) {
      var renderer = ref.renderer;

      return (renderer.listitem = function (text) {
      var isTaskItem = /^(<input.*type="checkbox"[^>]*>)/.test(text);
      var html = isTaskItem
        ? ("<li class=\"task-list-item\"><label>" + text + "</label></li>")
        : ("<li>" + text + "</li>");

      return html;
    });
  };

  var linkCompiler = function (ref) {
      var renderer = ref.renderer;
      var router = ref.router;
      var linkTarget = ref.linkTarget;
      var linkRel = ref.linkRel;
      var compilerClass = ref.compilerClass;

      return (renderer.link = function (href, title, text) {
      if ( title === void 0 ) title = '';

      var attrs = [];
      var ref = getAndRemoveConfig(title);
      var str = ref.str;
      var config = ref.config;
      linkTarget = config.target || linkTarget;
      linkRel =
        linkTarget === '_blank'
          ? compilerClass.config.externalLinkRel || 'noopener'
          : '';
      title = str;

      if (
        !isAbsolutePath(href) &&
        !compilerClass._matchNotCompileLink(href) &&
        !config.ignore
      ) {
        if (href === compilerClass.config.homepage) {
          href = 'README';
        }

        href = router.toURL(href, null, router.getCurrentPath());
      } else {
        if (!isAbsolutePath(href) && href.slice(0, 2) === './') {
          href =
            document.URL.replace(/\/(?!.*\/).*/, '/').replace('#/./', '') + href;
        }
        attrs.push(href.indexOf('mailto:') === 0 ? '' : ("target=\"" + linkTarget + "\""));
        attrs.push(
          href.indexOf('mailto:') === 0
            ? ''
            : linkRel !== ''
            ? (" rel=\"" + linkRel + "\"")
            : ''
        );
      }

      // special case to check crossorigin urls
      if (
        config.crossorgin &&
        linkTarget === '_self' &&
        compilerClass.config.routerMode === 'history'
      ) {
        if (compilerClass.config.crossOriginLinks.indexOf(href) === -1) {
          compilerClass.config.crossOriginLinks.push(href);
        }
      }

      if (config.disabled) {
        attrs.push('disabled');
        href = 'javascript:void(0)';
      }

      if (config.class) {
        attrs.push(("class=\"" + (config.class) + "\""));
      }

      if (config.id) {
        attrs.push(("id=\"" + (config.id) + "\""));
      }

      if (title) {
        attrs.push(("title=\"" + title + "\""));
      }

      return ("<a href=\"" + href + "\" " + (attrs.join(' ')) + ">" + text + "</a>");
    });
  };

  var cachedLinks = {};

  var compileMedia = {
    markdown: function markdown(url) {
      return {
        url: url,
      };
    },
    mermaid: function mermaid(url) {
      return {
        url: url,
      };
    },
    iframe: function iframe(url, title) {
      return {
        html: ("<iframe src=\"" + url + "\" " + (title ||
          'width=100% height=400') + "></iframe>"),
      };
    },
    video: function video(url, title) {
      return {
        html: ("<video src=\"" + url + "\" " + (title || 'controls') + ">Not Support</video>"),
      };
    },
    audio: function audio(url, title) {
      return {
        html: ("<audio src=\"" + url + "\" " + (title || 'controls') + ">Not Support</audio>"),
      };
    },
    code: function code(url, title) {
      var lang = url.match(/\.(\w+)$/);

      lang = title || (lang && lang[1]);
      if (lang === 'md') {
        lang = 'markdown';
      }

      return {
        url: url,
        lang: lang,
      };
    },
  };

  var Compiler = function Compiler(config, router) {
    var this$1 = this;

    this.config = config;
    this.router = router;
    this.cacheTree = {};
    this.toc = [];
    this.cacheTOC = {};
    this.linkTarget = config.externalLinkTarget || '_blank';
    this.linkRel =
      this.linkTarget === '_blank' ? config.externalLinkRel || 'noopener' : '';
    this.contentBase = router.getBasePath();

    var renderer = this._initRenderer();
    this.heading = renderer.heading;
    var compile;
    var mdConf = config.markdown || {};

    if (isFn(mdConf)) {
      compile = mdConf(marked_1, renderer);
    } else {
      marked_1.setOptions(
        merge(mdConf, {
          renderer: merge(renderer, mdConf.renderer),
        })
      );
      compile = marked_1;
    }

    this._marked = compile;
    this.compile = function (text) {
      var isCached = true;
      // eslint-disable-next-line no-unused-vars
      var result = cached(function (_) {
        isCached = false;
        var html = '';

        if (!text) {
          return text;
        }

        if (isPrimitive(text)) {
          html = compile(text);
        } else {
          html = compile.parser(text);
        }

        html = config.noEmoji ? html : emojify(html);
        slugify.clear();

        return html;
      })(text);

      var curFileName = this$1.router.parse().file;

      if (isCached) {
        this$1.toc = this$1.cacheTOC[curFileName];
      } else {
        this$1.cacheTOC[curFileName] = [].concat( this$1.toc );
      }

      return result;
    };
  };

  /**
   * Pulls content from file and renders inline on the page as a embedded item.
   *
   * This allows you to embed different file types on the returned
   * page.
   * The basic format is:
   * ```
   * [filename](_media/example.md ':include')
   * ```
   *
   * @param {string} href The href to the file to embed in the page.
   * @param {string} titleTitle of the link used to make the embed.
   *
   * @return {type} Return value description.
   */
  Compiler.prototype.compileEmbed = function compileEmbed (href, title) {
    var ref = getAndRemoveConfig(title);
      var str = ref.str;
      var config = ref.config;
    var embed;
    title = str;

    if (config.include) {
      if (!isAbsolutePath(href)) {
        href = getPath(
           this.contentBase,
          getParentPath(this.router.getCurrentPath()),
          href
        );
      }

      var media;
      if (config.type && (media = compileMedia[config.type])) {
        embed = media.call(this, href, title);
        embed.type = config.type;
      } else {
        var type = 'code';
        if (/\.(md|markdown)/.test(href)) {
          type = 'markdown';
        } else if (/\.mmd/.test(href)) {
          type = 'mermaid';
        } else if (/\.html?/.test(href)) {
          type = 'iframe';
        } else if (/\.(mp4|ogg)/.test(href)) {
          type = 'video';
        } else if (/\.mp3/.test(href)) {
          type = 'audio';
        }

        embed = compileMedia[type].call(this, href, title);
        embed.type = type;
      }

      embed.fragment = config.fragment;

      return embed;
    }
  };

  Compiler.prototype._matchNotCompileLink = function _matchNotCompileLink (link) {
    var links = this.config.noCompileLinks || [];

    for (var i = 0; i < links.length; i++) {
      var n = links[i];
      var re = cachedLinks[n] || (cachedLinks[n] = new RegExp(("^" + n + "$")));

      if (re.test(link)) {
        return link;
      }
    }
  };

  Compiler.prototype._initRenderer = function _initRenderer () {
    var renderer = new marked_1.Renderer();
    var ref = this;
      var linkTarget = ref.linkTarget;
      var linkRel = ref.linkRel;
      var router = ref.router;
      var contentBase = ref.contentBase;
    var _self = this;
    var origin = {};

    /**
     * Render anchor tag
     * @link https://github.com/markedjs/marked#overriding-renderer-methods
     * @param {String} text Text content
     * @param {Number} level Type of heading (h<level> tag)
     * @returns {String} Heading element
     */
    origin.heading = renderer.heading = function(text, level) {
      var ref = getAndRemoveConfig(text);
        var str = ref.str;
        var config = ref.config;
      var nextToc = { level: level, title: removeAtag(str) };

      if (/<!-- {docsify-ignore} -->/g.test(str)) {
        str = str.replace('<!-- {docsify-ignore} -->', '');
        nextToc.title = removeAtag(str);
        nextToc.ignoreSubHeading = true;
      }

      if (/{docsify-ignore}/g.test(str)) {
        str = str.replace('{docsify-ignore}', '');
        nextToc.title = removeAtag(str);
        nextToc.ignoreSubHeading = true;
      }

      if (/<!-- {docsify-ignore-all} -->/g.test(str)) {
        str = str.replace('<!-- {docsify-ignore-all} -->', '');
        nextToc.title = removeAtag(str);
        nextToc.ignoreAllSubs = true;
      }

      if (/{docsify-ignore-all}/g.test(str)) {
        str = str.replace('{docsify-ignore-all}', '');
        nextToc.title = removeAtag(str);
        nextToc.ignoreAllSubs = true;
      }

      var slug = slugify(config.id || str);
      var url = router.toURL(router.getCurrentPath(), { id: slug });
      nextToc.slug = url;
      _self.toc.push(nextToc);

      return ("<h" + level + " id=\"" + slug + "\"><a href=\"" + url + "\" data-id=\"" + slug + "\" class=\"anchor\"><span>" + str + "</span></a></h" + level + ">");
    };

    origin.code = highlightCodeCompiler({ renderer: renderer });
    origin.link = linkCompiler({
      renderer: renderer,
      router: router,
      linkTarget: linkTarget,
      linkRel: linkRel,
      compilerClass: _self,
    });
    origin.paragraph = paragraphCompiler({ renderer: renderer });
    origin.image = imageCompiler({ renderer: renderer, contentBase: contentBase, router: router });
    origin.list = taskListCompiler({ renderer: renderer });
    origin.listitem = taskListItemCompiler({ renderer: renderer });

    renderer.origin = origin;

    return renderer;
  };

  /**
   * Compile sidebar
   * @param {String} text Text content
   * @param {Number} level Type of heading (h<level> tag)
   * @returns {String} Sidebar element
   */
  Compiler.prototype.sidebar = function sidebar (text, level) {
    var ref = this;
      var toc = ref.toc;
    var currentPath = this.router.getCurrentPath();
    var html = '';

    if (text) {
      html = this.compile(text);
    } else {
      for (var i = 0; i < toc.length; i++) {
        if (toc[i].ignoreSubHeading) {
          var deletedHeaderLevel = toc[i].level;
          toc.splice(i, 1);
          // Remove headers who are under current header
          for (
            var j = i;
            j < toc.length && deletedHeaderLevel < toc[j].level;
            j++
          ) {
            toc.splice(j, 1) && j-- && i++;
          }

          i--;
        }
      }

      var tree$1 = this.cacheTree[currentPath] || genTree(toc, level);
      html = tree(tree$1, '<ul>{inner}</ul>');
      this.cacheTree[currentPath] = tree$1;
    }

    return html;
  };

  /**
   * Compile sub sidebar
   * @param {Number} level Type of heading (h<level> tag)
   * @returns {String} Sub-sidebar element
   */
  Compiler.prototype.subSidebar = function subSidebar (level) {
    if (!level) {
      this.toc = [];
      return;
    }

    var currentPath = this.router.getCurrentPath();
    var ref = this;
      var cacheTree = ref.cacheTree;
      var toc = ref.toc;

    toc[0] && toc[0].ignoreAllSubs && toc.splice(0);
    toc[0] && toc[0].level === 1 && toc.shift();

    for (var i = 0; i < toc.length; i++) {
      toc[i].ignoreSubHeading && toc.splice(i, 1) && i--;
    }

    var tree$1 = cacheTree[currentPath] || genTree(toc, level);

    cacheTree[currentPath] = tree$1;
    this.toc = [];
    return tree(tree$1);
  };

  Compiler.prototype.header = function header (text, level) {
    return this.heading(text, level);
  };

  Compiler.prototype.article = function article (text) {
    return this.compile(text);
  };

  /**
   * Compile cover page
   * @param {Text} text Text content
   * @returns {String} Cover page
   */
  Compiler.prototype.cover = function cover (text) {
    var cacheToc = this.toc.slice();
    var html = this.compile(text);

    this.toc = cacheToc.slice();

    return html;
  };

  var minIndent = function (string) {
  	var match = string.match(/^[ \t]*(?=\S)/gm);

  	if (!match) {
  		return 0;
  	}

  	return match.reduce(function (r, a) { return Math.min(r, a.length); }, Infinity);
  };

  var stripIndent = function (string) {
  	var indent = minIndent(string);

  	if (indent === 0) {
  		return string;
  	}

  	var regex = new RegExp(("^[ \\t]{" + indent + "}"), 'gm');

  	return string.replace(regex, '');
  };

  var cached$2 = {};

  function walkFetchEmbed(ref, cb) {
    var embedTokens = ref.embedTokens;
    var compile = ref.compile;
    var fetch = ref.fetch;

    var token;
    var step = 0;
    var count = 1;

    if (!embedTokens.length) {
      return cb({});
    }

    while ((token = embedTokens[step++])) {
      // eslint-disable-next-line no-shadow
      var next = (function(token) {
        return function (text) {
          var embedToken;
          if (text) {
            if (token.embed.type === 'markdown') {
              var path = token.embed.url.split('/');
              path.pop();
              path = path.join('/');
              // Resolves relative links to absolute
              text = text.replace(/\[([^[\]]+)\]\(([^)]+)\)/g, function (x) {
                var linkBeginIndex = x.indexOf('(');
                if (x.slice(linkBeginIndex, linkBeginIndex + 2) === '(.') {
                  return (
                    x.substring(0, linkBeginIndex) +
                    "(" + (window.location.protocol) + "//" + (window.location.host) + path + "/" +
                    x.substring(linkBeginIndex + 1, x.length - 1) +
                    ')'
                  );
                }
                return x;
              });

              // This may contain YAML front matter and will need to be stripped.
              var frontMatterInstalled =
                ($docsify.frontMatter || {}).installed || false;
              if (frontMatterInstalled === true) {
                text = $docsify.frontMatter.parseMarkdown(text);
              }

              embedToken = compile.lexer(text);
            } else if (token.embed.type === 'code') {
              if (token.embed.fragment) {
                var fragment = token.embed.fragment;
                var pattern = new RegExp(
                  ("(?:###|\\/\\/\\/)\\s*\\[" + fragment + "\\]([\\s\\S]*)(?:###|\\/\\/\\/)\\s*\\[" + fragment + "\\]")
                );
                text = stripIndent((text.match(pattern) || [])[1] || '').trim();
              }

              embedToken = compile.lexer(
                '```' +
                  token.embed.lang +
                  '\n' +
                  text.replace(/`/g, '@DOCSIFY_QM@') +
                  '\n```\n'
              );
            } else if (token.embed.type === 'mermaid') {
              embedToken = [
                { type: 'html', text: ("<div class=\"mermaid\">\n" + text + "\n</div>") } ];
              embedToken.links = {};
            } else {
              embedToken = [{ type: 'html', text: text }];
              embedToken.links = {};
            }
          }

          cb({ token: token, embedToken: embedToken });
          if (++count >= step) {
            cb({});
          }
        };
      })(token);

      if (token.embed.url) {
        {
          get(token.embed.url).then(next);
        }
      } else {
        next(token.embed.html);
      }
    }
  }

  function prerenderEmbed(ref, done) {
    var compiler = ref.compiler;
    var raw = ref.raw; if ( raw === void 0 ) raw = '';
    var fetch = ref.fetch;

    var hit = cached$2[raw];
    if (hit) {
      var copy = hit.slice();
      copy.links = hit.links;
      return done(copy);
    }

    var compile = compiler._marked;
    var tokens = compile.lexer(raw);
    var embedTokens = [];
    var linkRE = compile.Lexer.rules.inline.link;
    var links = tokens.links;

    tokens.forEach(function (token, index) {
      if (token.type === 'paragraph') {
        token.text = token.text.replace(
          new RegExp(linkRE.source, 'g'),
          function (src, filename, href, title) {
            var embed = compiler.compileEmbed(href, title);

            if (embed) {
              embedTokens.push({
                index: index,
                embed: embed,
              });
            }

            return src;
          }
        );
      }
    });

    // keep track of which tokens have been embedded so far
    // so that we know where to insert the embedded tokens as they
    // are returned
    var moves = [];
    walkFetchEmbed({ compile: compile, embedTokens: embedTokens, fetch: fetch }, function (ref) {
      var embedToken = ref.embedToken;
      var token = ref.token;

      if (token) {
        // iterate through the array of previously inserted tokens
        // to determine where the current embedded tokens should be inserted
        var index = token.index;
        moves.forEach(function (pos) {
          if (index > pos.start) {
            index += pos.length;
          }
        });

        merge(links, embedToken.links);

        tokens = tokens
          .slice(0, index)
          .concat(embedToken, tokens.slice(index + 1));
        moves.push({ start: index, length: embedToken.length - 1 });
      } else {
        cached$2[raw] = tokens.concat();
        tokens.links = cached$2[raw].links = links;
        done(tokens);
      }
    });
  }

  /* eslint-disable no-unused-vars */

  var vueGlobalData;

  function executeScript() {
    var script = findAll('.markdown-section>script')
      .filter(function (s) { return !/template/.test(s.type); })[0];
    if (!script) {
      return false;
    }

    var code = script.innerText.trim();
    if (!code) {
      return false;
    }

    new Function(code)();
  }

  function formatUpdated(html, updated, fn) {
    updated =
      typeof fn === 'function'
        ? fn(updated)
        : typeof fn === 'string'
        ? tinydate(fn)(new Date(updated))
        : updated;

    return html.replace(/{docsify-updated}/g, updated);
  }

  function renderMain(html) {
    var docsifyConfig = this.config;
    var markdownElm = find('.markdown-section');
    var vueVersion =
      'Vue' in window &&
      window.Vue.version &&
      Number(window.Vue.version.charAt(0));

    var isMountedVue = function (elm) {
      var isVue2 = Boolean(elm.__vue__ && elm.__vue__._isVue);
      var isVue3 = Boolean(elm._vnode && elm._vnode.__v_skip);

      return isVue2 || isVue3;
    };

    if (!html) {
      html = '<h1>404 - Not found</h1>';
    }

    if ('Vue' in window) {
      var mountedElms = findAll('.markdown-section > *')
        .filter(function (elm) { return isMountedVue(elm); });

      // Destroy/unmount existing Vue instances
      for (var i = 0, list = mountedElms; i < list.length; i += 1) {
        var mountedElm = list[i];

        if (vueVersion === 2) {
          mountedElm.__vue__.$destroy();
        } else if (vueVersion === 3) {
          mountedElm.__vue_app__.unmount();
        }
      }
    }

    this._renderTo(markdownElm, html);

    // Render sidebar with the TOC
    !docsifyConfig.loadSidebar && this._renderSidebar();

    // Execute markdown <script>
    if (
      docsifyConfig.executeScript ||
      ('Vue' in window && docsifyConfig.executeScript !== false)
    ) {
      executeScript();
    }

    // Handle Vue content not mounted by markdown <script>
    if ('Vue' in window) {
      var vueMountData = [];
      var vueComponentNames = Object.keys(docsifyConfig.vueComponents || {});

      // Register global vueComponents
      if (vueVersion === 2 && vueComponentNames.length) {
        vueComponentNames.forEach(function (name) {
          var isNotRegistered = !window.Vue.options.components[name];

          if (isNotRegistered) {
            window.Vue.component(name, docsifyConfig.vueComponents[name]);
          }
        });
      }

      // Store global data() return value as shared data object
      if (
        !vueGlobalData &&
        docsifyConfig.vueGlobalOptions &&
        typeof docsifyConfig.vueGlobalOptions.data === 'function'
      ) {
        vueGlobalData = docsifyConfig.vueGlobalOptions.data();
      }

      // vueMounts
      vueMountData.push.apply(
        vueMountData, Object.keys(docsifyConfig.vueMounts || {})
          .map(function (cssSelector) { return [
            find(markdownElm, cssSelector),
            docsifyConfig.vueMounts[cssSelector] ]; })
          .filter(function (ref) {
            var elm = ref[0];
            var vueConfig = ref[1];

            return elm;
      })
      );

      // Template syntax, vueComponents, vueGlobalOptions
      if (docsifyConfig.vueGlobalOptions || vueComponentNames.length) {
        var reHasBraces = /{{2}[^{}]*}{2}/;
        // Matches Vue full and shorthand syntax as attributes in HTML tags.
        //
        // Full syntax examples:
        // v-foo, v-foo[bar], v-foo-bar, v-foo:bar-baz.prop
        //
        // Shorthand syntax examples:
        // @foo, @foo.bar, @foo.bar.baz, @[foo], :foo, :[foo]
        //
        // Markup examples:
        // <div v-html>{{ html }}</div>
        // <div v-text="msg"></div>
        // <div v-bind:text-content.prop="text">
        // <button v-on:click="doThis"></button>
        // <button v-on:click.once="doThis"></button>
        // <button v-on:[event]="doThis"></button>
        // <button @click.stop.prevent="doThis">
        // <a :href="url">
        // <a :[key]="url">
        var reHasDirective = /<[^>/]+\s([@:]|v-)[\w-:.[\]]+[=>\s]/;

        vueMountData.push.apply(
          vueMountData, findAll('.markdown-section > *')
            // Remove duplicates
            .filter(function (elm) { return !vueMountData.some(function (ref) {
              var e = ref[0];
              var c = ref[1];

              return e === elm;
              }); })
            // Detect Vue content
            .filter(function (elm) {
              var isVueMount =
                // is a component
                elm.tagName.toLowerCase() in
                  (docsifyConfig.vueComponents || {}) ||
                // has a component(s)
                elm.querySelector(vueComponentNames.join(',') || null) ||
                // has curly braces
                reHasBraces.test(elm.outerHTML) ||
                // has content directive
                reHasDirective.test(elm.outerHTML);

              return isVueMount;
            })
            .map(function (elm) {
              // Clone global configuration
              var vueConfig = merge({}, docsifyConfig.vueGlobalOptions || {});

              // Replace vueGlobalOptions data() return value with shared data object.
              // This provides a global store for all Vue instances that receive
              // vueGlobalOptions as their configuration.
              if (vueGlobalData) {
                vueConfig.data = function() {
                  return vueGlobalData;
                };
              }

              return [elm, vueConfig];
            })
        );
      }

      // Mount
      for (var i$1 = 0, list$1 = vueMountData; i$1 < list$1.length; i$1 += 1) {
        var ref = list$1[i$1];
        var mountElm = ref[0];
        var vueConfig = ref[1];

        var isVueAttr = 'data-isvue';
        var isSkipElm =
          // Is an invalid tag
          mountElm.matches('pre, script') ||
          // Is a mounted instance
          isMountedVue(mountElm) ||
          // Has mounted instance(s)
          mountElm.querySelector(("[" + isVueAttr + "]"));

        if (!isSkipElm) {
          mountElm.setAttribute(isVueAttr, '');

          if (vueVersion === 2) {
            vueConfig.el = undefined;
            new window.Vue(vueConfig).$mount(mountElm);
          } else if (vueVersion === 3) {
            var app = window.Vue.createApp(vueConfig);

            // Register global vueComponents
            vueComponentNames.forEach(function (name) {
              var config = docsifyConfig.vueComponents[name];

              app.component(name, config);
            });

            app.mount(mountElm);
          }
        }
      }
    }
  }

  function renderNameLink(vm) {
    var el = getNode('.app-name-link');
    var nameLink = vm.config.nameLink;
    var path = vm.route.path;

    if (!el) {
      return;
    }

    if (isPrimitive(vm.config.nameLink)) {
      el.setAttribute('href', nameLink);
    } else if (typeof nameLink === 'object') {
      var match = Object.keys(nameLink).filter(
        function (key) { return path.indexOf(key) > -1; }
      )[0];

      el.setAttribute('href', nameLink[match]);
    }
  }

  /** @typedef {import('../Docsify').Constructor} Constructor */

  /**
   * @template {!Constructor} T
   * @param {T} Base - The class to extend
   */
  function Render(Base) {
    return /*@__PURE__*/(function (Base) {
      function Render () {
        Base.apply(this, arguments);
      }

      if ( Base ) Render.__proto__ = Base;
      Render.prototype = Object.create( Base && Base.prototype );
      Render.prototype.constructor = Render;

      Render.prototype._renderTo = function _renderTo (el, content, replace) {
        var node = getNode(el);
        if (node) {
          node[replace ? 'outerHTML' : 'innerHTML'] = content;
        }
      };

      Render.prototype._renderSidebar = function _renderSidebar (text) {
        var ref = this.config;
        var maxLevel = ref.maxLevel;
        var subMaxLevel = ref.subMaxLevel;
        var loadSidebar = ref.loadSidebar;
        var hideSidebar = ref.hideSidebar;

        if (hideSidebar) {
          // FIXME : better styling solution
          [
            document.querySelector('aside.sidebar'),
            document.querySelector('button.sidebar-toggle') ].forEach(function (node) { return node.parentNode.removeChild(node); });
          document.querySelector('section.content').style.right = 'unset';
          document.querySelector('section.content').style.left = 'unset';
          document.querySelector('section.content').style.position = 'relative';
          document.querySelector('section.content').style.width = '100%';
          return null;
        }

        this._renderTo('.sidebar-nav', this.compiler.sidebar(text, maxLevel));
        var activeEl = getAndActive(this.router, '.sidebar-nav', true, true);
        if (loadSidebar && activeEl) {
          activeEl.parentNode.innerHTML +=
            this.compiler.subSidebar(subMaxLevel) || '';
        } else {
          // Reset toc
          this.compiler.subSidebar();
        }

        // Bind event
        this._bindEventOnRendered(activeEl);
      };

      Render.prototype._bindEventOnRendered = function _bindEventOnRendered (activeEl) {
        var ref = this.config;
        var autoHeader = ref.autoHeader;

        scrollActiveSidebar(this.router);

        if (autoHeader && activeEl) {
          var main = getNode('#main');
          var firstNode = main.children[0];
          if (firstNode && firstNode.tagName !== 'H1') {
            var h1 = this.compiler.header(activeEl.innerText, 1);
            var wrapper = create('div', h1);
            before(main, wrapper.children[0]);
          }
        }
      };

      Render.prototype._renderNav = function _renderNav (text) {
        text && this._renderTo('nav', this.compiler.compile(text));
        if (this.config.loadNavbar) {
          getAndActive(this.router, 'nav');
        }
      };

      Render.prototype._renderMain = function _renderMain (text, opt, next) {
        var this$1 = this;
        if ( opt === void 0 ) opt = {};

        if (!text) {
          return renderMain.call(this, text);
        }

        this.callHook('beforeEach', text, function (result) {
          var html;
          var callback = function () {
            if (opt.updatedAt) {
              html = formatUpdated(
                html,
                opt.updatedAt,
                this$1.config.formatUpdated
              );
            }

            this$1.callHook('afterEach', html, function (hookData) { return renderMain.call(this$1, hookData); }
            );
          };

          if (this$1.isHTML) {
            html = this$1.result = text;
            callback();
            next();
          } else {
            prerenderEmbed(
              {
                compiler: this$1.compiler,
                raw: result,
              },
              function (tokens) {
                html = this$1.compiler.compile(tokens);
                html = this$1.isRemoteUrl
                  ? purify.sanitize(html, { ADD_TAGS: ['script'] })
                  : html;
                callback();
                next();
              }
            );
          }
        });
      };

      Render.prototype._renderCover = function _renderCover (text, coverOnly) {
        var el = getNode('.cover');

        toggleClass(
          getNode('main'),
          coverOnly ? 'add' : 'remove',
          'hidden'
        );
        if (!text) {
          toggleClass(el, 'remove', 'show');
          return;
        }

        toggleClass(el, 'add', 'show');

        var html = this.coverIsHTML ? text : this.compiler.cover(text);

        var m = html
          .trim()
          .match('<p><img.*?data-origin="(.*?)"[^a]+alt="(.*?)">([^<]*?)</p>$');

        if (m) {
          if (m[2] === 'color') {
            el.style.background = m[1] + (m[3] || '');
          } else {
            var path = m[1];

            toggleClass(el, 'add', 'has-mask');
            if (!isAbsolutePath(m[1])) {
              path = getPath(this.router.getBasePath(), m[1]);
            }

            el.style.backgroundImage = "url(" + path + ")";
            el.style.backgroundSize = 'cover';
            el.style.backgroundPosition = 'center center';
          }

          html = html.replace(m[0], '');
        }

        this._renderTo('.cover-main', html);
        sticky();
      };

      Render.prototype._updateRender = function _updateRender () {
        // Render name link
        renderNameLink(this);
      };

      Render.prototype.initRender = function initRender () {
        var config = this.config;

        // Init markdown compiler
        this.compiler = new Compiler(config, this.router);
        {
          /* eslint-disable-next-line camelcase */
          window.__current_docsify_compiler__ = this.compiler;
        }

        var id = config.el || '#app';
        var navEl = find('nav') || create('nav');

        var el = find(id);
        var html = '';
        var navAppendToTarget = body;

        if (el) {
          if (config.repo) {
            html += corner(config.repo, config.cornerExternalLinkTarge);
          }

          if (config.coverpage) {
            html += cover();
          }

          if (config.logo) {
            var isBase64 = /^data:image/.test(config.logo);
            var isExternal = /(?:http[s]?:)?\/\//.test(config.logo);
            var isRelative = /^\./.test(config.logo);

            if (!isBase64 && !isExternal && !isRelative) {
              config.logo = getPath(this.router.getBasePath(), config.logo);
            }
          }

          html += main(config);
          // Render main app
          this._renderTo(el, html, true);
        } else {
          this.rendered = true;
        }

        if (config.mergeNavbar && isMobile) {
          navAppendToTarget = find('.sidebar');
        } else {
          navEl.classList.add('app-nav');

          if (!config.repo) {
            navEl.classList.add('no-badge');
          }
        }

        // Add nav
        if (config.loadNavbar) {
          before(navAppendToTarget, navEl);
        }

        if (config.themeColor) {
          $.head.appendChild(
            create('div', theme(config.themeColor)).firstElementChild
          );
          // Polyfll
          cssVars(config.themeColor);
        }

        this._updateRender();
        toggleClass(body, 'ready');
      };

      return Render;
    }(Base));
  }

  /* eslint-disable no-unused-vars */

  function loadNested(path, qs, file, next, vm, first) {
    path = first ? path : path.replace(/\/$/, '');
    path = getParentPath(path);

    if (!path) {
      return;
    }

    get(
      vm.router.getFile(path + file) + qs,
      false,
      vm.config.requestHeaders
    ).then(next, function (_) { return loadNested(path, qs, file, next, vm); });
  }

  /** @typedef {import('../Docsify').Constructor} Constructor */

  /**
   * @template {!Constructor} T
   * @param {T} Base - The class to extend
   */
  function Fetch(Base) {
    var last;

    var abort = function () { return last && last.abort && last.abort(); };
    var request = function (url, hasbar, requestHeaders) {
      abort();
      last = get(url, true, requestHeaders);
      return last;
    };

    var get404Path = function (path, config) {
      var notFoundPage = config.notFoundPage;
      var ext = config.ext;
      var defaultPath = '_404' + (ext || '.md');
      var key;
      var path404;

      switch (typeof notFoundPage) {
        case 'boolean':
          path404 = defaultPath;
          break;
        case 'string':
          path404 = notFoundPage;
          break;

        case 'object':
          key = Object.keys(notFoundPage)
            .sort(function (a, b) { return b.length - a.length; })
            .filter(function (k) { return path.match(new RegExp('^' + k)); })[0];

          path404 = (key && notFoundPage[key]) || defaultPath;
          break;
      }

      return path404;
    };

    return /*@__PURE__*/(function (Base) {
      function Fetch () {
        Base.apply(this, arguments);
      }

      if ( Base ) Fetch.__proto__ = Base;
      Fetch.prototype = Object.create( Base && Base.prototype );
      Fetch.prototype.constructor = Fetch;

      Fetch.prototype._loadSideAndNav = function _loadSideAndNav (path, qs, loadSidebar, cb) {
        var this$1 = this;

        return function () {
          if (!loadSidebar) {
            return cb();
          }

          var fn = function (result) {
            this$1._renderSidebar(result);
            cb();
          };

          // Load sidebar
          loadNested(path, qs, loadSidebar, fn, this$1, true);
        };
      };

      Fetch.prototype._fetch = function _fetch (cb) {
        var this$1 = this;
        if ( cb === void 0 ) cb = noop;

        var ref = this.route;
        var query = ref.query;
        var ref$1 = this.route;
        var path = ref$1.path;

        // Prevent loading remote content via URL hash
        // Ex: https://foo.com/#//bar.com/file.md
        if (isExternal(path)) {
          history.replaceState(null, '', '#');
          this.router.normalize();
        } else {
          var qs = stringifyQuery(query, ['id']);
          var ref$2 = this.config;
          var loadNavbar = ref$2.loadNavbar;
          var requestHeaders = ref$2.requestHeaders;
          var loadSidebar = ref$2.loadSidebar;
          // Abort last request

          var file = this.router.getFile(path);
          var req = request(file + qs, true, requestHeaders);

          this.isRemoteUrl = isExternal(file);
          // Current page is html
          this.isHTML = /\.html$/g.test(file);

          // Load main content
          req.then(
            function (text, opt) { return this$1._renderMain(
                text,
                opt,
                this$1._loadSideAndNav(path, qs, loadSidebar, cb)
              ); },
            function (_) {
              this$1._fetchFallbackPage(path, qs, cb) ||
                this$1._fetch404(file, qs, cb);
            }
          );

          // Load nav
          loadNavbar &&
            loadNested(
              path,
              qs,
              loadNavbar,
              function (text) { return this$1._renderNav(text); },
              this,
              true
            );
        }
      };

      Fetch.prototype._fetchCover = function _fetchCover () {
        var this$1 = this;

        var ref = this.config;
        var coverpage = ref.coverpage;
        var requestHeaders = ref.requestHeaders;
        var query = this.route.query;
        var root = getParentPath(this.route.path);

        if (coverpage) {
          var path = null;
          var routePath = this.route.path;
          if (typeof coverpage === 'string') {
            if (routePath === '/') {
              path = coverpage;
            }
          } else if (Array.isArray(coverpage)) {
            path = coverpage.indexOf(routePath) > -1 && '_coverpage';
          } else {
            var cover = coverpage[routePath];
            path = cover === true ? '_coverpage' : cover;
          }

          var coverOnly = Boolean(path) && this.config.onlyCover;
          if (path) {
            path = this.router.getFile(root + path);
            this.coverIsHTML = /\.html$/g.test(path);
            get(
              path + stringifyQuery(query, ['id']),
              false,
              requestHeaders
            ).then(function (text) { return this$1._renderCover(text, coverOnly); });
          } else {
            this._renderCover(null, coverOnly);
          }

          return coverOnly;
        }
      };

      Fetch.prototype.$fetch = function $fetch (cb, $resetEvents) {
        var this$1 = this;
        if ( cb === void 0 ) cb = noop;
        if ( $resetEvents === void 0 ) $resetEvents = this.$resetEvents.bind(this);

        var done = function () {
          this$1.callHook('doneEach');
          cb();
        };

        var onlyCover = this._fetchCover();

        if (onlyCover) {
          done();
        } else {
          this._fetch(function () {
            $resetEvents();
            done();
          });
        }
      };

      Fetch.prototype._fetchFallbackPage = function _fetchFallbackPage (path, qs, cb) {
        var this$1 = this;
        if ( cb === void 0 ) cb = noop;

        var ref = this.config;
        var requestHeaders = ref.requestHeaders;
        var fallbackLanguages = ref.fallbackLanguages;
        var loadSidebar = ref.loadSidebar;

        if (!fallbackLanguages) {
          return false;
        }

        var local = path.split('/')[1];

        if (fallbackLanguages.indexOf(local) === -1) {
          return false;
        }

        var newPath = this.router.getFile(
          path.replace(new RegExp(("^/" + local)), '')
        );
        var req = request(newPath + qs, true, requestHeaders);

        req.then(
          function (text, opt) { return this$1._renderMain(
              text,
              opt,
              this$1._loadSideAndNav(path, qs, loadSidebar, cb)
            ); },
          function () { return this$1._fetch404(path, qs, cb); }
        );

        return true;
      };

      /**
       * Load the 404 page
       * @param {String} path URL to be loaded
       * @param {*} qs TODO: define
       * @param {Function} cb Callback
       * @returns {Boolean} True if the requested page is not found
       * @private
       */
      Fetch.prototype._fetch404 = function _fetch404 (path, qs, cb) {
        var this$1 = this;
        if ( cb === void 0 ) cb = noop;

        var ref = this.config;
        var loadSidebar = ref.loadSidebar;
        var requestHeaders = ref.requestHeaders;
        var notFoundPage = ref.notFoundPage;

        var fnLoadSideAndNav = this._loadSideAndNav(path, qs, loadSidebar, cb);
        if (notFoundPage) {
          var path404 = get404Path(path, this.config);

          request(this.router.getFile(path404), true, requestHeaders).then(
            function (text, opt) { return this$1._renderMain(text, opt, fnLoadSideAndNav); },
            function () { return this$1._renderMain(null, {}, fnLoadSideAndNav); }
          );
          return true;
        }

        this._renderMain(null, {}, fnLoadSideAndNav);
        return false;
      };

      Fetch.prototype.initFetch = function initFetch () {
        var this$1 = this;

        var ref = this.config;
        var loadSidebar = ref.loadSidebar;

        // Server-Side Rendering
        if (this.rendered) {
          var activeEl = getAndActive(this.router, '.sidebar-nav', true, true);
          if (loadSidebar && activeEl) {
            activeEl.parentNode.innerHTML += window.__SUB_SIDEBAR__;
          }

          this._bindEventOnRendered(activeEl);
          this.$resetEvents();
          this.callHook('doneEach');
          this.callHook('ready');
        } else {
          this.$fetch(function (_) { return this$1.callHook('ready'); });
        }
      };

      return Fetch;
    }(Base));
  }

  /** @typedef {import('../Docsify').Constructor} Constructor */

  /**
   * @template {!Constructor} T
   * @param {T} Base - The class to extend
   */
  function Events(Base) {
    return /*@__PURE__*/(function (Base) {
      function Events () {
        Base.apply(this, arguments);
      }

      if ( Base ) Events.__proto__ = Base;
      Events.prototype = Object.create( Base && Base.prototype );
      Events.prototype.constructor = Events;

      Events.prototype.$resetEvents = function $resetEvents (source) {
        var this$1 = this;

        var ref = this.config;
        var auto2top = ref.auto2top;

        (function () {
          // Rely on the browser's scroll auto-restoration when going back or forward
          if (source === 'history') {
            return;
          }
          // Scroll to ID if specified
          if (this$1.route.query.id) {
            scrollIntoView(this$1.route.path, this$1.route.query.id);
          }
          // Scroll to top if a link was clicked and auto2top is enabled
          if (source === 'navigate') {
            auto2top && scroll2Top(auto2top);
          }
        })();

        if (this.config.loadNavbar) {
          getAndActive(this.router, 'nav');
        }
      };

      Events.prototype.initEvent = function initEvent () {
        // Bind toggle button
        btn('button.sidebar-toggle', this.router);
        collapse('.sidebar', this.router);
        // Bind sticky effect
        if (this.config.coverpage) {
          !isMobile && on('scroll', sticky);
        } else {
          body.classList.add('sticky');
        }
      };

      return Events;
    }(Base));
  }



  var util = /*#__PURE__*/Object.freeze({
    __proto__: null,
    cached: cached,
    hyphenate: hyphenate,
    hasOwn: hasOwn,
    merge: merge,
    isPrimitive: isPrimitive,
    noop: noop,
    isFn: isFn,
    isExternal: isExternal,
    inBrowser: inBrowser,
    isMobile: isMobile,
    supportsPushState: supportsPushState,
    parseQuery: parseQuery,
    stringifyQuery: stringifyQuery,
    isAbsolutePath: isAbsolutePath,
    removeParams: removeParams,
    getParentPath: getParentPath,
    cleanPath: cleanPath,
    resolvePath: resolvePath,
    getPath: getPath,
    replaceSlug: replaceSlug,
    endsWith: endsWith
  });

  // TODO This is deprecated, kept for backwards compatibility. Remove in next
  // major release. We'll tell people to get everything from the DOCSIFY global
  // when using the global build, but we'll highly recommend for them to import
  // from the ESM build (f.e. lib/docsify.esm.js and lib/docsify.min.esm.js).
  function initGlobalAPI() {
    window.Docsify = {
      util: util,
      dom: dom,
      get: get,
      slugify: slugify,
      version: '4.12.2',
    };
    window.DocsifyCompiler = Compiler;
    window.marked = marked_1;
    window.Prism = prism;
  }

  /** @typedef {import('../Docsify').Constructor} Constructor */

  /**
   * @template {!Constructor} T
   * @param {T} Base - The class to extend
   */
  function Lifecycle(Base) {
    return /*@__PURE__*/(function (Base) {
      function Lifecycle () {
        Base.apply(this, arguments);
      }

      if ( Base ) Lifecycle.__proto__ = Base;
      Lifecycle.prototype = Object.create( Base && Base.prototype );
      Lifecycle.prototype.constructor = Lifecycle;

      Lifecycle.prototype.initLifecycle = function initLifecycle () {
        var this$1 = this;

        var hooks = [
          'init',
          'mounted',
          'beforeEach',
          'afterEach',
          'doneEach',
          'ready' ];

        this._hooks = {};
        this._lifecycle = {};

        hooks.forEach(function (hook) {
          var arr = (this$1._hooks[hook] = []);
          this$1._lifecycle[hook] = function (fn) { return arr.push(fn); };
        });
      };

      Lifecycle.prototype.callHook = function callHook (hookName, data, next) {
        if ( next === void 0 ) next = noop;

        var queue = this._hooks[hookName];

        var step = function(index) {
          var hookFn = queue[index];

          if (index >= queue.length) {
            next(data);
          } else if (typeof hookFn === 'function') {
            if (hookFn.length === 2) {
              hookFn(data, function (result) {
                data = result;
                step(index + 1);
              });
            } else {
              var result = hookFn(data);
              data = result === undefined ? data : result;
              step(index + 1);
            }
          } else {
            step(index + 1);
          }
        };

        step(0);
      };

      return Lifecycle;
    }(Base));
  }

  /** @typedef {new (...args: any[]) => any} Constructor */

  // eslint-disable-next-line new-cap
  var Docsify = /*@__PURE__*/(function (superclass) {
    function Docsify() {
      superclass.call(this);

      this.config = config(this);

      this.initLifecycle(); // Init hooks
      this.initPlugin(); // Install plugins
      this.callHook('init');
      this.initRouter(); // Add router
      this.initRender(); // Render base DOM
      this.initEvent(); // Bind events
      this.initFetch(); // Fetch data
      this.callHook('mounted');
    }

    if ( superclass ) Docsify.__proto__ = superclass;
    Docsify.prototype = Object.create( superclass && superclass.prototype );
    Docsify.prototype.constructor = Docsify;

    Docsify.prototype.initPlugin = function initPlugin () {
      var this$1 = this;

      []
        .concat(this.config.plugins)
        .forEach(function (fn) { return isFn(fn) && fn(this$1._lifecycle, this$1); });
    };

    return Docsify;
  }(Fetch(Events(Render(Router(Lifecycle(Object)))))));

  /**
   * Global API
   */
  initGlobalAPI();

  /**
   * Run Docsify
   */
  // eslint-disable-next-line no-unused-vars
  documentReady(function (_) { return new Docsify(); });

}());
