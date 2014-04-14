var Gogits = {
    "PageIsSignup": false
};

(function ($) {
    // extend jQuery ajax, set csrf token value
    var ajax = $.ajax;
    $.extend({
        ajax: function (url, options) {
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
                headers = $.extend(headers, {'X-Csrf-Token': csrftoken});
            }
            options.headers = headers;
            var callback = options.success;
            options.success = function (data) {
                if (data.once) {
                    // change all _once value if ajax data.once exist
                    $('[name=_once]').val(data.once);
                }
                if (callback) {
                    callback.apply(this, arguments);
                }
            };
            return ajax(url, options);
        },

        changeHash: function (hash) {
            if (history.pushState) {
                history.pushState(null, null, hash);
            }
            else {
                location.hash = hash;
            }
        },

        deSelect: function () {
            if (window.getSelection) {
                window.getSelection().removeAllRanges();
            } else {
                document.selection.empty();
            }
        }
    });
    $.fn.extend({
        toggleHide: function () {
            $(this).addClass("hidden");
        },
        toggleShow: function () {
            $(this).removeClass("hidden");
        },
        toggleAjax: function (successCallback) {
            var url = $(this).data("ajax");
            var method = $(this).data('ajax-method') || 'get';
            var ajaxName = $(this).data('ajax-name');
            var data = {};
            $('[data-ajax-rel=' + ajaxName + ']').each(function () {
                var field = $(this).data("ajax-field");
                var t = $(this).data("ajax-val");
                if (t == "val") {
                    data[field] = $(this).val();
                    return true;
                }
                if (t == "txt") {
                    data[field] = $(this).text();
                    return true;
                }
                if (t == "html") {
                    data[field] = $(this).html();
                    return true;
                }
                if (t == "data") {
                    data[field] = $(this).data("ajax-data");
                    return true;
                }
                return true;
            });
            $.ajax({
                url: url,
                method: method.toUpperCase(),
                data: data,
                success: function (d) {
                    if (successCallback) {
                        successCallback(d);
                    }
                }
            })
        }
    })
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
        $tabs.tab("show");
        $tabs.find("li:eq(0) a").tab("show");
    };

    // fix dropdown inside click
    Gogits.initDropDown = function () {
        $('.dropdown-menu.no-propagation').on('click', function (e) {
            e.stopPropagation();
        });
    };


    // render markdown
    Gogits.renderMarkdown = function () {
        var $md = $('.markdown');
        var $pre = $md.find('pre > code').parent();
        $pre.addClass('prettyprint linenums');
        prettyPrint();

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
    };

    // render code view
    Gogits.renderCodeView = function () {
        function selectRange($list, $select, $from) {
            $list.removeClass('active');
            if ($from) {
                var a = parseInt($select.attr('rel').substr(1));
                var b = parseInt($from.attr('rel').substr(1));
                var c;
                if (a != b) {
                    if (a > b) {
                        c = a;
                        a = b;
                        b = c;
                    }
                    var classes = [];
                    for (i = a; i <= b; i++) {
                        classes.push('.L' + i);
                    }
                    $list.filter(classes.join(',')).addClass('active');
                    $.changeHash('#L' + a + '-' + 'L' + b);
                    return
                }
            }
            $select.addClass('active');
            $.changeHash('#' + $select.attr('rel'));
        }

        $(document).on('click', '.lines-num span', function (e) {
            var $select = $(this);
            var $list = $select.parent().siblings('.lines-code').find('ol.linenums > li');
            selectRange($list, $list.filter('[rel=' + $select.attr('rel') + ']'), (e.shiftKey ? $list.filter('.active').eq(0) : null));
            $.deSelect();
        });

        $('.code-view .lines-code > pre').each(function () {
            var $pre = $(this);
            var $lineCode = $pre.parent();
            var $lineNums = $lineCode.siblings('.lines-num');
            if ($lineNums.length > 0) {
                var nums = $pre.find('ol.linenums > li').length;
                for (var i = 1; i <= nums; i++) {
                    $lineNums.append('<span id="L' + i + '" rel="L' + i + '">' + i + '</span>');
                }
            }
        });

        $(window).on('hashchange',function (e) {
            var m = window.location.hash.match(/^#(L\d+)\-(L\d+)$/);
            var $list = $('.code-view ol.linenums > li');
            if (m) {
                var $first = $list.filter('.' + m[1]);
                selectRange($list, $first, $list.filter('.' + m[2]));
                $("html, body").scrollTop($first.offset().top - 200);
                return;
            }
            m = window.location.hash.match(/^#(L\d+)$/);
            if (m) {
                var $first = $list.filter('.' + m[1]);
                selectRange($list, $first);
                $("html, body").scrollTop($first.offset().top - 200);
            }
        }).trigger('hashchange');
    };

    // copy utils
    Gogits.bindCopy = function (selector) {
        if ($(selector).hasClass('js-copy-bind')) {
            return;
        }
        $(selector).zclip({
            path: "/js/ZeroClipboard.swf",
            copy: function () {
                var t = $(this).data("copy-val");
                var to = $($(this).data("copy-from"));
                var str = "";
                if (t == "txt") {
                    str = to.text();
                }
                if (t == 'val') {
                    str = to.val();
                }
                if (t == 'html') {
                    str = to.html();
                }
                return str;
            },
            afterCopy: function () {
                var $this = $(this);
                $this.tooltip('hide')
                    .attr('data-original-title', 'Copied OK');
                setTimeout(function () {
                    $this.tooltip("show");
                }, 200);
                setTimeout(function () {
                    $this.tooltip('hide')
                        .attr('data-original-title', 'Copy to Clipboard');
                }, 3000);
            }
        }).addClass("js-copy-bind");
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
    Gogits.renderCodeView();
}

function initRegister() {
    $.getScript("/js/jquery.validate.min.js", function () {
        Gogits.validateForm("#login-card", {
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
    // ssh confirmation
    $('#ssh-keys .delete').confirmation({
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

    // profile form
    (function () {
        $('#user-setting-username').on("keyup", function () {
            var $this = $(this);
            if ($this.val() != $this.attr('title')) {
                $this.next('.help-block').toggleShow();
            } else {
                $this.next('.help-block').toggleHide();
            }
        });
    }())
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
                    $clone.find('.input-group-btn .btn-primary').removeClass('btn-primary').addClass("btn-default");
                    $(this).addClass('btn-primary').removeClass('btn-default');
                    $url.val($this.data("link"));
                    $clone.find('span.clone-url').text($this.data('link'));
                }
            }).eq(0).trigger("click");
            $("#repo-clone").on("shown.bs.dropdown", function () {
                Gogits.bindCopy("[data-init=copy]");
            });
            Gogits.bindCopy("[data-init=copy]:visible");
        }
    })();

    // watching script
    (function () {
        var $watch = $('#repo-watching'),
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

    // repo diff counter
    (function () {
        var $counter = $('.diff-counter');
        if ($counter.length < 1) {
            return;
        }
        $counter.each(function (i, item) {
            var $item = $(item);
            var addLine = $item.find('span[data-line].add').data("line");
            var delLine = $item.find('span[data-line].del').data("line");
            var addPercent = parseFloat(addLine) / (parseFloat(addLine) + parseFloat(delLine)) * 100;
            $item.find(".bar .add").css("width", addPercent + "%");
        });
    }());

    // repo setting form
    (function () {
        $('#repo-setting-name').on("keyup", function () {
            var $this = $(this);
            if ($this.val() != $this.attr('title')) {
                $this.next('.help-block').toggleShow();
            } else {
                $this.next('.help-block').toggleHide();
            }
        });
    }())
}

function initInstall() {
    // database type change
    (function () {
        $('#install-database').on("change", function () {
            var val = $(this).val();
            if (val != "sqlite") {
                $('.server-sql').show();
                $('.sqlite-setting').addClass("hide");
                if (val == "pgsql") {
                    $('.pgsql-setting').removeClass("hide");
                } else {
                    $('.pgsql-setting').addClass("hide");
                }
            } else {
                $('.server-sql').hide();
                $('.sqlite-setting').removeClass("hide");
            }
        });
    }());

}

function initIssue() {
    // close button
    (function () {
        var $closeBtn = $('#issue-close-btn');
        var $openBtn = $('#issue-open-btn');
        $('#issue-reply-content').on("keyup", function () {
            if ($(this).val().length) {
                $closeBtn.val($closeBtn.data("text"));
                $openBtn.val($openBtn.data("text"));
            } else {
                $closeBtn.val($closeBtn.data("origin"));
                $openBtn.val($openBtn.data("origin"));
            }
        });
    }());

    // issue edit mode
    (function () {
        $("#issue-edit-btn").on("click", function () {
            $('#issue h1.title,#issue .issue-main > .issue-content .content,#issue-edit-btn').toggleHide();
            $('#issue-edit-title,#issue-edit-content,.issue-edit-cancel,.issue-edit-save').toggleShow();
        });
        $('.issue-edit-cancel').on("click", function () {
            $('#issue h1.title,#issue .issue-main > .issue-content .content,#issue-edit-btn').toggleShow();
            $('#issue-edit-title,#issue-edit-content,.issue-edit-cancel,.issue-edit-save').toggleHide();
        })
    }());

    // issue ajax update
    (function () {
        $('.issue-edit-save').on("click", function () {
            $(this).toggleAjax(function (json) {
                if (json.ok) {
                    $('.issue-head h1.title').text(json.title);
                    $('.issue-main > .issue-content .content').html(json.content);
                    $('.issue-edit-cancel').trigger("click");
                }
            });
        });
    }());

    // issue ajax preview
    (function () {
        $('[data-ajax-name=issue-preview]').on("click", function () {
            var $this = $(this);
            $this.toggleAjax(function (json) {
                if (json.ok) {
                    $($this.data("preview")).html(json.content);
                }
            })
        });
        $('.issue-write a[data-toggle]').on("click", function () {
            $('.issue-preview-content').html("loading...");
        });
    }())

}

function initRelease() {
// release new ajax preview
    (function () {
        $('[data-ajax-name=release-preview]').on("click", function () {
            var $this = $(this);
            $this.toggleAjax(function (json) {
                if (json.ok) {
                    $($this.data("preview")).html(json.content);
                }
            })
        });
        $('.release-write a[data-toggle]').on("click", function () {
            $('.release-preview-content').html("loading...");
        });
    }());

    // release new target selection
    (function () {
        $('#release-new-target-branch-list').on('click', 'a', function () {
            $('#tag-target').val($(this).text());
            $('#release-new-target-name').text(" " + $(this).text());
        });
    }());
}

(function ($) {
    $(function () {
        initCore();
        var body = $("#body");
        if (body.data("page") == "user-signup") {
            initRegister();
        }
        if (body.data("page") == "user") {
            initUserSetting();
        }
        if ($('.repo-nav').length) {
            initRepository();
        }
        if ($('#install-card').length) {
            initInstall();
        }
        if ($('#issue').length) {
            initIssue();
        }
        if ($('#release').length) {
            initRelease();
        }
    });
})(jQuery);
