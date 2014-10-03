/*
 js tabs and tabbed content plugin
 */
function Tabs(selector) {

    function hide($nav) {
        console.log("hide", $nav);
        $nav.removeClass("js-tab-nav-show");
        $($nav.data("tab-target")).removeClass("js-tab-show").hide();
    }

    function show($nav) {
        console.log("show", $nav);
        $nav.addClass("js-tab-nav-show");
        $($nav.data("tab-target")).addClass("js-tab-show").show();
    }

    var $e = $(selector);
    if ($e.length) {
        // pre-assign init index
        var $current = $e.find('.js-tab-nav-show');
        if ($current.length) {
            $($current.data("tab-target")).addClass("js-tab-show");
        }
        // bind nav click
        $e.on("click", ".js-tab-nav", function (e) {
            e.preventDefault();
            var $this = $(this);
            // is showing, not change.
            if ($this.hasClass("js-tab-nav-show")) {
                return;
            }
            $current = $e.find(".js-tab-nav-show").eq(0);
            hide($current);
            show($this);
        });
        console.log("init tabs @", selector)
    }
}

$.fn.extend({
    tabs: function () {
        Tabs(this);
    }
});