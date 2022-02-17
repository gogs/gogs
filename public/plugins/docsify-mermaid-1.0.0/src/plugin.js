let plugin = (hook, vm) => {

    hook.afterEach(function (html, next) {
        // We load the HTML inside a DOM node to allow for manipulation
        var htmlElement = document.createElement('div');
        htmlElement.innerHTML = html;

        htmlElement.querySelectorAll('pre[data-lang=mermaid]').forEach((element) => {
            // Create a <div class="mermaid"> to replace the <pre> 
            var replacement = document.createElement('div');
            replacement.textContent = element.textContent;
            replacement.classList.add('mermaid');

            // Replace
            element.parentNode.replaceChild(replacement, element);
        });

        next(htmlElement.innerHTML);
    });

    hook.doneEach(function () {
        mermaid.init({}, '.mermaid');
    });

};

export default plugin;