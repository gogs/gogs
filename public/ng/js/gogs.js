// @codekit-prepend "lib/jquery-1.11.1.min.js"
// @codekit-prepend "lib/lib.js"
// @codekit-prepend "lib/tabs.js"

var Gogs = {};

(function ($) {
    // Extend jQuery ajax, set CSRF token value.
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
}(jQuery));

(function ($) {
    // Render markdown.
    Gogs.renderMarkdown = function () {
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

    // Render code view.
    Gogs.renderCodeView = function () {
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

        $(window).on('hashchange', function (e) {
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
})(jQuery);

function initCore() {
    Gogs.renderMarkdown();
    Gogs.renderCodeView();
}

function initRepoCreate() {
    // Owner switch menu click.
    $('#repo-create-owner-list').on('click', 'li', function () {
        if (!$(this).hasClass('checked')) {
            var uid = $(this).data('uid');
            $('#repo-owner-id').val(uid);
            $('#repo-owner-avatar').attr("src", $(this).find('img').attr("src"));
            $('#repo-owner-name').text($(this).text().trim());

            $(this).parent().find('.checked').removeClass('checked');
            $(this).addClass('checked');
            console.log("set repo owner to uid :", uid, $(this).text().trim());
        }
    });
}

$(document).ready(function () {
    initCore();
    if ($('#repo-create-form').length) {
        initRepoCreate();
    }

    Tabs('#dashboard-sidebar-menu');

    homepage();
    settingsProfile();
    settingsSSHKeys();
    settingsDelete();

    // Fix language drop-down menu height.
    var l = $('#footer-lang li').length;
    $('#footer-lang .drop-down').css({
        "top": (-31 * l) + "px",
        "height": (31 * l - 3) + "px"
    });
});

function homepage() {
    // Change method to GET if no username input.
    $('#promo-form').submit(function (e) {
        if ($('#username').val() === "") {
            e.preventDefault();
            window.location.href = '/user/login';
            return true
        }
    });
    // Redirect to register page.
    $('#register-button').click(function (e) {
        if ($('#username').val() === "") {
            e.preventDefault();
            window.location.href = '/user/sign_up';
            return true
        }
        $('#promo-form').attr('action', '/user/sign_up');
    });
}

function settingsProfile() {
    // Confirmation of change username in user profile page.
    $('#user-profile-form').submit(function (e) {
        if (($('#username').data('uname') != $('#username').val()) && !confirm('Username has been changed, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });
}

function settingsSSHKeys() {
    // Show add SSH key panel.
    $('#ssh-add').click(function () {
        $('#user-ssh-add-form').removeClass("hide");
    });
}

function settingsDelete() {
    // Confirmation of delete account.
    $('#delete-account-button').click(function (e) {
        if (!confirm('This account is going to deleted, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });
}