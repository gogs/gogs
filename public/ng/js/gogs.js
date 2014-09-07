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
    $.fn.extend({
        toggleHide: function () {
            $(this).addClass("hidden");
        },
        toggleShow: function () {
            $(this).removeClass("hidden");
        },
        toggleAjax: function (successCallback, errorCallback) {
            var url = $(this).data("ajax");
            var method = $(this).data('ajax-method') || 'get';
            var ajaxName = $(this).data('ajax-name');
            var data = {};

            if (ajaxName.endsWith("preview")) {
                data["mode"] = "gfm";
                data["context"] = $(this).data('ajax-context');
            }

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
            console.log("toggleAjax:", method, url, data);
            $.ajax({
                url: url,
                method: method.toUpperCase(),
                data: data,
                error: errorCallback,
                success: function (d) {
                    if (successCallback) {
                        successCallback(d);
                    }
                }
            })
        }
    });
}(jQuery));

(function ($) {
    // Render markdown.
    Gogs.renderMarkdown = function () {
        var $md = $('.markdown');
        var $pre = $md.find('pre > code').parent();
        $pre.addClass('prettyprint');
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
            var $first;
            if (m) {
                $first = $list.filter('.' + m[1]);
                selectRange($list, $first, $list.filter('.' + m[2]));
                $("html, body").scrollTop($first.offset().top - 200);
                return;
            }
            m = window.location.hash.match(/^#(L\d+)$/);
            if (m) {
                $first = $list.filter('.' + m[1]);
                selectRange($list, $first);
                $("html, body").scrollTop($first.offset().top - 200);
            }
        }).trigger('hashchange');
    };

    // Search users by keyword.
    Gogs.searchUsers = function (val, $target) {
        $.ajax({
            url: '/api/v1/users/search?q=' + val,
            dataType: "json",
            success: function (json) {
                if (json.ok && json.data.length) {
                    var html = '';
                    $.each(json.data, function (i, item) {
                        html += '<li><a><img src="' + item.avatar + '">' + item.username + '</a></li>';
                    });
                    $target.html(html);
                    $target.toggleShow();
                } else {
                    $target.toggleHide();
                }
            }
        });
    }

    // Search repositories by keyword.
    Gogs.searchRepos = function (val, $target, $param) {
        $.ajax({
            url: '/api/v1/repos/search?q=' + val + '&' + $param,
            dataType: "json",
            success: function (json) {
                if (json.ok && json.data.length) {
                    var html = '';
                    $.each(json.data, function (i, item) {
                        html += '<li><a><span class="octicon octicon-repo"></span> ' + item.repolink + '</a></li>';
                    });
                    $target.html(html);
                    $target.toggleShow();
                } else {
                    $target.toggleHide();
                }
            }
        });
    }

    // Copy util.
    Gogs.bindCopy = function (selector) {
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
                alert("Clone URL has copied!");
//                var $this = $(this);
//                $this.tooltip('hide')
//                    .attr('data-original-title', 'Copied OK');
//                setTimeout(function () {
//                    $this.tooltip("show");
//                }, 200);
//                setTimeout(function () {
//                    $this.tooltip('hide')
//                        .attr('data-original-title', 'Copy to Clipboard');
//                }, 3000);
            }
        }).addClass("js-copy-bind");
    }
})(jQuery);

function initCore() {
    Gogs.renderMarkdown();
    Gogs.renderCodeView();
}

function initUserSetting() {
    // Confirmation of change username in user profile page.
    $('#user-profile-form').submit(function (e) {
        var $username = $('#username');
        if (($username.data('uname') != $username.val()) && !confirm('Username has been changed, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });

    // Show add SSH key panel.
    $('#ssh-add').click(function () {
        $('#user-ssh-add-form').removeClass("hide");
    });

    // Confirmation of delete account.
    $('#delete-account-button').click(function (e) {
        if (!confirm('This account is going to be deleted, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });
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

    $('#auth-button').click(function (e) {
        $('#repo-migrate-auth').slideToggle('fast');
        e.preventDefault();
    })
    console.log('initRepoCreate');
}

function initRepo() {
    // Clone link switch button.
    $('#repo-clone-ssh').click(function () {
        $(this).removeClass('btn-gray').addClass('btn-blue');
        $('#repo-clone-https').removeClass('btn-blue').addClass('btn-gray');
        $('#repo-clone-url').val($(this).data('link'));
        $('.clone-url').text($(this).data('link'))
    });
    $('#repo-clone-https').click(function () {
        $(this).removeClass('btn-gray').addClass('btn-blue');
        $('#repo-clone-ssh').removeClass('btn-blue').addClass('btn-gray');
        $('#repo-clone-url').val($(this).data('link'));
        $('.clone-url').text($(this).data('link'))
    });
    // Copy URL.
    $('#repo-clone-copy').hover(function () {
        Gogs.bindCopy($(this));
    })
}

// when user changes hook type, hide/show proper divs
function initHookTypeChange() {
    // web hook type change
    $('select#hook-type').on("change", function () {
      hookTypes = ['Gogs','Slack'];

      var curHook = $(this).val();
      hookTypes.forEach(function(hookType) {
        if (curHook === hookType) {
          $('div#'+hookType.toLowerCase()).toggleShow();
        }
        else {
          $('div#'+hookType.toLowerCase()).toggleHide();
        }
      });
    });
}

function initRepoSetting() {
    // Options.
    // Confirmation of changing repository name.
    $('#repo-setting-form').submit(function (e) {
        var $reponame = $('#repo_name');
        if (($reponame.data('repo-name') != $reponame.val()) && !confirm('Repository name has been changed, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });

    initHookTypeChange();

    $('#transfer-button').click(function () {
        $('#transfer-form').show();
    });
    $('#delete-button').click(function () {
        $('#delete-form').show();
    });

    // Collaboration.
    $('#repo-collab-list hr:last-child').remove();
    var $ul = $('#repo-collaborator').next().next().find('ul');
    $('#repo-collaborator').on('keyup', function () {
        var $this = $(this);
        if (!$this.val()) {
            $ul.toggleHide();
            return;
        }
        Gogs.searchUsers($this.val(), $ul);
    }).on('focus', function () {
        if (!$(this).val()) {
            $ul.toggleHide();
        } else {
            $ul.toggleShow();
        }
    }).next().next().find('ul').on("click", 'li', function () {
        $('#repo-collaborator').val($(this).text());
        $ul.toggleHide();
    });
}

function initOrgSetting() {
    // Options.
    // Confirmation of changing organization name.
    $('#org-setting-form').submit(function (e) {
        var $orgname = $('#orgname');
        if (($orgname.data('orgname') != $orgname.val()) && !confirm('Organization name has been changed, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });
    // Confirmation of delete organization.
    $('#delete-org-button').click(function (e) {
        if (!confirm('This organization is going to be deleted, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
    });

    initHookTypeChange();
}

function initInvite() {
    // Invitation.
    var $ul = $('#org-member-invite-list');
    $('#org-member-invite').on('keyup', function () {
        var $this = $(this);
        if (!$this.val()) {
            $ul.toggleHide();
            return;
        }
        Gogs.searchUsers($this.val(), $ul);
    }).on('focus', function () {
        if (!$(this).val()) {
            $ul.toggleHide();
        } else {
            $ul.toggleShow();
        }
    }).next().next().find('ul').on("click", 'li', function () {
        $('#org-member-invite').val($(this).text());
        $ul.toggleHide();
    });
}

function initOrgTeamCreate() {
    // Delete team.
    $('#org-team-delete').click(function (e) {
        if (!confirm('This team is going to be deleted, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
        var $form = $('#team-create-form');
        $form.attr('action', $form.data('delete-url'));
    });
}

function initTeamMembersList() {
    // Add team member.
    var $ul = $('#org-team-members-list');
    $('#org-team-members-add').on('keyup', function () {
        var $this = $(this);
        if (!$this.val()) {
            $ul.toggleHide();
            return;
        }
        Gogs.searchUsers($this.val(), $ul);
    }).on('focus', function () {
        if (!$(this).val()) {
            $ul.toggleHide();
        } else {
            $ul.toggleShow();
        }
    }).next().next().find('ul').on("click", 'li', function () {
        $('#org-team-members-add').val($(this).text());
        $ul.toggleHide();
    });
}

function initTeamRepositoriesList() {
    // Add team repository.
    var $ul = $('#org-team-repositories-list');
    $('#org-team-repositories-add').on('keyup', function () {
        var $this = $(this);
        if (!$this.val()) {
            $ul.toggleHide();
            return;
        }
        Gogs.searchRepos($this.val(), $ul, 'uid=' + $this.data('uid'));
    }).on('focus', function () {
        if (!$(this).val()) {
            $ul.toggleHide();
        } else {
            $ul.toggleShow();
        }
    }).next().next().find('ul').on("click", 'li', function () {
        $('#org-team-repositories-add').val($(this).text());
        $ul.toggleHide();
    });
}

function initAdmin() {
    // Create account.
    $('#login-type').on("change", function () {
        var v = $(this).val();
        if (v.indexOf("0-") + 1) {
            $('.auth-name').toggleHide();
            $(".pwd").find("input").attr("required", "required")
                .end().toggleShow();
        } else {
            $(".pwd").find("input").removeAttr("required")
                .end().toggleHide();
            $('.auth-name').toggleShow();
        }
    });
    // Delete account.
    $('#user-delete').click(function (e) {
        if (!confirm('This account is going to be deleted, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
        var $form = $('#user-profile-form');
        $form.attr('action', $form.data('delete-url'));
    });
    // Create authorization.
    $('#auth-type').on("change", function () {
        var v = $(this).val();
        if (v == 2) {
            $('.ldap').toggleShow();
            $('.smtp').toggleHide();
        }
        if (v == 3) {
            $('.smtp').toggleShow();
            $('.ldap').toggleHide();
        }
    });
    // Delete authorization.
    $('#auth-delete').click(function (e) {
        if (!confirm('This authorization is going to be deleted, do you want to continue?')) {
            e.preventDefault();
            return true;
        }
        var $form = $('auth-setting-form');
        $form.attr('action', $form.data('delete-url'));
    });
}

function initInstall() {
    // Change database type.
    (function () {
        var mysql_default = '127.0.0.1:3306';
        var postgres_default = '127.0.0.1:5432';

        $('#install-database').on("change", function () {
            var val = $(this).val();
            if (val != "SQLite3") {
                $('.server-sql').show();
                $('.sqlite-setting').addClass("hide");
                if (val == "PostgreSQL") {
                    $('.pgsql-setting').removeClass("hide");

                    // Change the host value to the Postgres default, but only
                    // if the user hasn't already changed it from the MySQL
                    // default.
                    if ($('#database-host').val() == mysql_default) {
                        $('#database-host').val(postgres_default);
                    }
                } else if (val == 'MySQL') {
                    $('.pgsql-setting').addClass("hide");
                    if ($('#database-host').val() == postgres_default) {
                        $('#database-host').val(mysql_default);
                    }
                } else {
                    $('.pgsql-setting').addClass("hide");
                }
            } else {
                $('.server-sql').hide();
                $('.pgsql-setting').hide();
                $('.sqlite-setting').removeClass("hide");
            }
        });
    }());
}

$(document).ready(function () {
    initCore();
    if ($('#user-profile-setting').length) {
        initUserSetting();
    }
    if ($('#repo-create-form').length || $('#repo-migrate-form').length) {
        initRepoCreate();
    }
    if ($('#repo-header').length) {
        initRepo();
    }
    if ($('#repo-setting').length) {
        initRepoSetting();
    }
    if ($('#org-setting').length) {
        initOrgSetting();
    }
    if ($('#invite-box').length) {
        initInvite();
    }
    if ($('#team-create-form').length) {
        initOrgTeamCreate();
    }
    if ($('#team-members-list').length) {
        initTeamMembersList();
    }
    if ($('#team-repositories-list').length) {
        initTeamRepositoriesList();
    }
    if ($('#admin-setting').length) {
        initAdmin();
    }
    if ($('#install-form').length) {
        initInstall();
    }

    Tabs('#dashboard-sidebar-menu');

    homepage();

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
