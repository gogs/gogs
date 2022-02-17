import plugin from './plugin';

if (!window.$docsify) {
    window.$docsify = {}
}

window.$docsify.plugins = (window.$docsify.plugins || []).concat(plugin)