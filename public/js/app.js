var Gogits = {};

(function($){
    Gogits.showTooltips = function(){
        $("body").tooltip({
            selector: "[data-toggle=tooltip]"
            //container: "body"
        });
    };
    Gogits.showTab = function (selector, index) {
        if (!index) {
            index = 0;
        }
        $(selector).tab("show");
        $(selector).find("li:eq(" + index + ") a").tab("show");
    }
})(jQuery);


function initCore(){
    Gogits.showTooltips();
}