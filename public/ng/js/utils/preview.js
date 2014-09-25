/**
 * preview plugin
 * @param selector
 * @param target_selector
 */
function Preview(selector, target_selector) {

    // get input element
    function get_input($e) {
        return $e.find(".js-preview-input").eq(0);
    }

    // get result html container element
    function get_container($t) {
        if ($t.hasClass("js-preview-container")) {
            return $t
        }
        return $t.find(".js-preview-container").eq(0);
    }

    var $e = $(selector);
    var $t = $(target_selector);

    var $ipt = get_input($t);
    if (!$ipt.length) {
        console.log("[preview]: no preview input");
        return
    }
    var $cnt = get_container($t);
    if (!$cnt.length) {
        console.log("[preview]: no preview container");
        return
    }


    // call api via ajax
    $e.on("click", function () {
        $.post("/api/v1/markdown", {
            text: $ipt.val()
        }, function (html) {
            $cnt.html(html);
        })
    });

    console.log("[preview]: init preview @", selector, "&", target_selector);
}


$.fn.extend({
    markdown_preview: function (target) {
        Preview(this, target);
    }
});
