var Gogits = {
    "PageIsSignup": false
};

(function ($) {
    Gogits.showTooltips = function () {
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
    };
    Gogits.validateForm = function (selector, options) {
        var $form = $(selector);
        options = options || {};
        options.showErrors = function (map, list) {
            var $error = $form.find('.form-error').addClass('hidden');
            $('.has-error').removeClass("has-error");
            $error.text(list[0].message).show().removeClass("hidden");
            $(list[0].element).parents(".form-group").addClass("has-error");
        };
        $form.validate(options);
    };
})(jQuery);


function initCore() {
    Gogits.showTooltips();
}

function initRegister() {
    $.getScript("/js/jquery.validate.min.js", function () {
        Gogits.validateForm("#gogs-login-card", {
            rules: {
                "username": {
                    required: true,
                    minlength: 5,
                    maxlength: 30
                },
                "email": {
                    required: true,
                    email: true
                },
                "passwd": {
                    required: true,
                    minlength: 6,
                    maxlength: 30
                },
                "re-passwd": {
                    required: true,
                    equalTo: "input[name=passwd]"
                }
            }
        });
    });
}