var Gogits = {
    "PageIsSignup": false
};

(function($){
    // extend jQuery ajax, set csrf token value
    var ajax = $.ajax;
    $.extend({
        ajax: function(url, options) {
            if (typeof url === 'object') {
                options = url;
                url = undefined;
            }
            options = options || {};
            url = options.url;
            var csrftoken = $('meta[name=_csrf]').attr('content');
            var headers = options.headers || {};
            var domain = document.domain.replace(/\./ig, '\\.');
            if (!/^(http:|https:).*/.test(url) || eval('/^(http:|https:)\\/\\/(.+\\.)*' + domain + '.*/').test(url)) {
                headers = $.extend(headers, {'X-Csrf-Token':csrftoken});
            }
            options.headers = headers;
            var callback = options.success;
            options.success = function(data){
                if(data.once){
                    // change all _once value if ajax data.once exist
                    $('[name=_once]').val(data.once);
                }
                if(callback){
                    callback.apply(this, arguments);
                }
            };
            return ajax(url, options);
        }
    });
}(jQuery));

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
    // fix dropdown inside click
    Gogits.initDropDown = function(){
        $('.dropdown-menu').on('click','a,button,input,select',function(e){
            e.stopPropagation();
        });
    };

    // render markdown
    Gogits.renderMarkdown = function () {
        var $md = $('.markdown');
        var $pre = $md.find('pre > code').parent();
        $pre.addClass('prettyprint linenums');
        prettyPrint();

        var $lineNums = $pre.parent().siblings('.lines-num');
        if ($lineNums.length > 0) {
            var nums = $pre.find('ol.linenums > li').length;
            for (var i = 1; i <= nums; i++) {
                $lineNums.append('<span id="L' + i + '" rel=".L' + i + '">' + i + '</span>');
            }

            var last;
            $(document).on('click', '.lines-num span', function () {
                var $e = $(this);
                if (last) {
                    last.removeClass('active');
                }
                last = $e.parent().siblings('.lines-code').find('ol.linenums > ' + $e.attr('rel'));
                last.addClass('active');
                window.location.href = '#' + $e.attr('id');
            });
        }

        // Set anchor.
        var headers = {};
        $md.find('h1, h2, h3, h4, h5, h6').each(function () {
            var node = $(this);
            var val = encodeURIComponent(node.text().toLowerCase().replace(/[^\w\- ]/g, '').replace(/[ ]/g, '-'));
            var name = val;
            if (headers[val] > 0) {
                name = val + '-' + headers[val];
            }
            if (headers[val] == undefined) {
                headers[val] = 1;
            } else {
                headers[val] += 1;
            }
            node = node.wrap('<div id="' + name + '" class="anchor-wrap" ></div>');
            node.append('<a class="anchor" href="#' + name + '"><span class="octicon octicon-link"></span></a>');
        });
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
    Gogits.initDropDown();
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
    // clone group button script
    (function () {
        var $clone = $('.clone-group-btn');
        if ($clone.length) {
            var $url = $('.clone-group-url');
            $clone.find('button[data-link]').on("click",function (e) {
                var $this = $(this);
                if (!$this.hasClass('btn-primary')) {
                    $clone.find('.btn-primary').removeClass('btn-primary').addClass("btn-default");
                    $(this).addClass('btn-primary').removeClass('btn-default');
                    $url.val($this.data("link"));
                    $clone.find('span.clone-url').text($this.data('link'));
                }
            }).eq(0).trigger("click");
            // todo copy to clipboard
        }
    })();

    // watching script
    (function () {
        var $watch = $('#gogs-repo-watching'),
            watchLink = $watch.data("watch"),
            unwatchLink = $watch.data("unwatch");
        $watch.on('click', '.to-watch',function () {
            if ($watch.hasClass("watching")) {
                return false;
            }
            $.get(watchLink, function (json) {
                if (json.ok) {
                    $watch.find('.text-primary').removeClass('text-primary');
                    $watch.find('.to-watch h4').addClass('text-primary');
                    $watch.find('.fa-eye-slash').removeClass('fa-eye-slash').addClass('fa-eye');
                    $watch.removeClass("no-watching").addClass("watching");
                }
            });
            return false;
        }).on('click', '.to-unwatch', function () {
            if ($watch.hasClass("no-watching")) {
                return false;
            }
            $.get(unwatchLink, function (json) {
                if (json.ok) {
                    $watch.find('.text-primary').removeClass('text-primary');
                    $watch.find('.to-unwatch h4').addClass('text-primary');
                    $watch.find('.fa-eye').removeClass('fa-eye').addClass('fa-eye-slash');
                    $watch.removeClass("watching").addClass("no-watching");
                }
            });
            return false;
        });
    })();
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
