var Gogits = {
    "PageIsSignup": false
};

(function ($) {

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

    // ----- init elements
    Gogits.initModals = function () {
        var modals = $("[data-toggle=modal]");
        if (modals.length < 1) {
            return;
        }
        $.each(modals, function (i, item) {
            var hide = $(item).data('modal');
            $(item).modal(hide ? hide : "hide");
        });
    };
    Gogits.initTooltips = function () {
        $("body").tooltip({
            selector: "[data-toggle=tooltip]"
            //container: "body"
        });
    };
    Gogits.initPopovers = function () {
        var hideAllPopovers = function () {
            $('[data-toggle=popover]').each(function () {
                $(this).popover('hide');
            });
        };

        $(document).on('click', function (e) {
            var $e = $(e.target);
            if ($e.data('toggle') == 'popover' || $e.parents("[data-toggle=popover], .popover").length > 0) {
                return;
            }
            hideAllPopovers();
        });

        $("body").popover({
            selector: "[data-toggle=popover]"
        });
    };
    Gogits.initTabs = function () {
        var $tabs = $('[data-init=tabs]');
        $tabs.find("li:eq(0) a").tab("show");
    };

    // render markdown
    Gogits.renderMarkdown = function () {
        var $pre = $('.markdown').find('pre > code').parent();
        $pre.addClass("prettyprint");
        prettyPrint();
    }

})(jQuery);

// ajax utils
(function ($) {
    Gogits.ajaxDelete = function (url, data, success) {
        data = data || {};
        data._method = "DELETE";
        $.ajax({
            url: url,
            data: data,
            method: "POST",
            dataType: "json",
            success: function (json) {
                if (success) {
                    success(json);
                }
            }
        })
    }
})(jQuery);


function initCore() {
    Gogits.initTooltips();
    Gogits.initPopovers();
    Gogits.initTabs();
    Gogits.initModals();
    Gogits.renderMarkdown();
}

function initRegister() {
    $.getScript("/js/jquery.validate.min.js", function () {
        Gogits.validateForm("#gogs-login-card", {
            rules: {
                "username": {
                    required: true,
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

function initUserSetting() {
    $('#gogs-ssh-keys .delete').confirmation({
        singleton: true,
        onConfirm: function (e, $this) {
            Gogits.ajaxDelete("", {"id": $this.data("del")}, function (json) {
                if (json.ok) {
                    window.location.reload();
                } else {
                    alert(json.err);
                }
            });
        }
    });
}

function initRepository() {
    var $guide = $('.guide-box');
    if ($guide.length) {
        var $url = $('#guide-clone-url');
        $guide.find('button[data-url]').on("click",function () {
            var $this = $(this);
            if (!$this.hasClass('btn-primary')) {
                $guide.find('.btn-primary').removeClass('btn-primary').addClass("btn-default");
                $(this).addClass('btn-primary').removeClass('btn-default');
                $url.val($this.data("url"));
                $guide.find('span.clone-url').text($this.data('url'));
            }
        }).eq(0).trigger("click");
        // todo copy to clipboard
    }
}

(function ($) {
    $(function () {
        initCore();
        var body = $("#gogs-body");
        if (body.data("page") == "user-signup") {
            initRegister();
        }
        if (body.data("page") == "user") {
            initUserSetting();
        }
        if ($('.gogs-repo-nav').length) {
            initRepository();
        }
    });
})(jQuery);
