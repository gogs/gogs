'use strict';

var csrf;
var suburl;

function initCommentPreviewTab($form) {
    var $tab_menu = $form.find('.tabular.menu');
    $tab_menu.find('.item').tab();
    $tab_menu.find('.item[data-tab="' + $tab_menu.data('preview') + '"]').click(function () {
        var $this = $(this);
        $.post($this.data('url'), {
                "_csrf": csrf,
                "mode": "gfm",
                "context": $this.data('context'),
                "text": $form.find('.tab.segment[data-tab="' + $tab_menu.data('write') + '"] textarea').val()
            },
            function (data) {
                var $preview_tab = $form.find('.tab.segment[data-tab="' + $tab_menu.data('preview') + '"]');
                $preview_tab.html(data);
                emojify.run($preview_tab[0]);
                $('pre code', $preview_tab[0]).each(function (i, block) {
                    hljs.highlightBlock(block);
                });
            }
        );
    });

    buttonsClickOnEnter();
}

function initCommentForm() {
    if ($('.comment.form').length == 0) {
        return
    }

    initCommentPreviewTab($('.comment.form'));

    // Labels
    var $list = $('.ui.labels.list');
    var $no_select = $list.find('.no-select');
    var $label_menu = $('.select-label .menu');
    var has_label_update_action = $label_menu.data('action') == 'update';

    function updateIssueMeta(url, action, id) {
        $.post(url, {
            "_csrf": csrf,
            "action": action,
            "id": id
        });
    }

    $label_menu.find('.item:not(.no-select)').click(function () {
        if ($(this).hasClass('checked')) {
            $(this).removeClass('checked');
            $(this).find('.octicon').removeClass('octicon-check');
            if (has_label_update_action) {
                updateIssueMeta($label_menu.data('update-url'), "detach", $(this).data('id'));
            }
        } else {
            $(this).addClass('checked');
            $(this).find('.octicon').addClass('octicon-check');
            if (has_label_update_action) {
                updateIssueMeta($label_menu.data('update-url'), "attach", $(this).data('id'));
            }
        }

        var label_ids = "";
        $(this).parent().find('.item').each(function () {
            if ($(this).hasClass('checked')) {
                label_ids += $(this).data('id') + ",";
                $($(this).data('id-selector')).removeClass('hide');
            } else {
                $($(this).data('id-selector')).addClass('hide');
            }
        });
        if (label_ids.length == 0) {
            $no_select.removeClass('hide');
        } else {
            $no_select.addClass('hide');
        }
        $($(this).parent().data('id')).val(label_ids);
        return false;
    });
    $label_menu.find('.no-select.item').click(function () {
        if (has_label_update_action) {
            updateIssueMeta($label_menu.data('update-url'), "clear", '');
        }

        $(this).parent().find('.item').each(function () {
            $(this).removeClass('checked');
            $(this).find('.octicon').removeClass('octicon-check');
        });

        $list.find('.item').each(function () {
            $(this).addClass('hide');
        });
        $no_select.removeClass('hide');
        $($(this).parent().data('id')).val('');
    });

    function selectItem(select_id, input_id) {
        var $menu = $(select_id + ' .menu');
        var $list = $('.ui' + select_id + '.list');
        var has_update_action = $menu.data('action') == 'update';

        $menu.find('.item:not(.no-select)').click(function () {
            $(this).parent().find('.item').each(function () {
                $(this).removeClass('selected active')
            });

            $(this).addClass('selected active');
            if (has_update_action) {
                updateIssueMeta($menu.data('update-url'), '', $(this).data('id'));
            }
            switch (input_id) {
                case '#milestone_id':
                    $list.find('.selected').html('<a class="item" href=' + $(this).data('href') + '>' +
                        $(this).text() + '</a>');
                    break;
                case '#assignee_id':
                    $list.find('.selected').html('<a class="item" href=' + $(this).data('href') + '>' +
                        '<img class="ui avatar image" src=' + $(this).data('avatar') + '>' +
                        $(this).text() + '</a>');
            }
            $('.ui' + select_id + '.list .no-select').addClass('hide');
            $(input_id).val($(this).data('id'));
        });
        $menu.find('.no-select.item').click(function () {
            $(this).parent().find('.item:not(.no-select)').each(function () {
                $(this).removeClass('selected active')
            });

            if (has_update_action) {
                updateIssueMeta($menu.data('update-url'), '', '');
            }

            $list.find('.selected').html('');
            $list.find('.no-select').removeClass('hide');
            $(input_id).val('');
        });
    }

    // Milestone and assignee
    selectItem('.select-milestone', '#milestone_id');
    selectItem('.select-assignee', '#assignee_id');
}

function initInstall() {
    if ($('.install').length == 0) {
        return;
    }

    // Database type change detection.
    $("#db_type").change(function () {
        var sqlite_default = 'data/gogs.db';
        var tidb_default = 'data/gogs_tidb';

        var db_type = $(this).val();
        if (db_type === "SQLite3" || db_type === "TiDB") {
            $('#sql_settings').hide();
            $('#pgsql_settings').hide();
            $('#sqlite_settings').show();

            if (db_type === "SQLite3" && $('#db_path').val() == tidb_default) {
                $('#db_path').val(sqlite_default);
            } else if (db_type === "TiDB" && $('#db_path').val() == sqlite_default) {
                $('#db_path').val(tidb_default);
            }
            return;
        }

        var mysql_default = '127.0.0.1:3306';
        var postgres_default = '127.0.0.1:5432';

        $('#sqlite_settings').hide();
        $('#sql_settings').show();
        if (db_type === "PostgreSQL") {
            $('#pgsql_settings').show();
            if ($('#db_host').val() == mysql_default) {
                $('#db_host').val(postgres_default);
            }
        } else {
            $('#pgsql_settings').hide();
            if ($('#db_host').val() == postgres_default) {
                $('#db_host').val(mysql_default);
            }
        }
    });

    $('#offline-mode input').change(function () {
        if ($(this).is(':checked')) {
            $('#disable-gravatar').checkbox('check');
        }
    });
    $('#disable-registration input').change(function () {
        if ($(this).is(':checked')) {
            $('#enable-captcha').checkbox('uncheck');
        }
    });
    $('#enable-captcha input').change(function () {
        if ($(this).is(':checked')) {
            $('#disable-registration').checkbox('uncheck');
        }
    });
}

function initRepository() {
    if ($('.repository').length == 0) {
        return;
    }

    function initFilterSearchDropdown(selector) {
        var $dropdown = $(selector);
        $dropdown.dropdown({
            fullTextSearch: true,
            onChange: function (text, value, $choice) {
                window.location.href = $choice.data('url');
                console.log($choice.data('url'))
            },
            message: {noResults: $dropdown.data('no-results')}
        });
    }

    // File list and commits
    if ($('.repository.file.list').length > 0 ||
        ('.repository.commits').length > 0) {
        initFilterSearchDropdown('.choose.reference .dropdown');

        $('.reference.column').click(function () {
            $('.choose.reference .scrolling.menu').css('display', 'none');
            $('.choose.reference .text').removeClass('black');
            $($(this).data('target')).css('display', 'block');
            $(this).find('.text').addClass('black');
            return false;
        });
    }

    // Wiki
    if ($('.repository.wiki.view').length > 0) {
        initFilterSearchDropdown('.choose.page .dropdown');
    }

    // Options
    if ($('.repository.settings.options').length > 0) {
        $('#repo_name').keyup(function () {
            var $prompt_span = $('#repo-name-change-prompt');
            if ($(this).val().toString().toLowerCase() != $(this).data('repo-name').toString().toLowerCase()) {
                $prompt_span.show();
            } else {
                $prompt_span.hide();
            }
        });
    }

    // Labels
    if ($('.repository.labels').length > 0) {
        // Create label
        var $new_label_panel = $('.new-label.segment');
        $('.new-label.button').click(function () {
            $new_label_panel.show();
        });
        $('.new-label.segment .cancel').click(function () {
            $new_label_panel.hide();
        });

        $('.color-picker').each(function () {
            $(this).minicolors();
        });
        $('.precolors .color').click(function () {
            var color_hex = $(this).data('color-hex');
            $('.color-picker').val(color_hex);
            $('.minicolors-swatch-color').css("background-color", color_hex);
        });
        $('.edit-label-button').click(function () {
            $('#label-modal-id').val($(this).data('id'));
            $('.edit-label .new-label-input').val($(this).data('title'));
            $('.edit-label .color-picker').val($(this).data('color'));
            $('.minicolors-swatch-color').css("background-color", $(this).data('color'));
            $('.edit-label.modal').modal({
                onApprove: function () {
                    $('.edit-label.form').submit();
                }
            }).modal('show');
            return false;
        });
    }

    // Milestones
    if ($('.repository.milestones').length > 0) {

    }
    if ($('.repository.new.milestone').length > 0) {
        var $datepicker = $('.milestone.datepicker');
        $datepicker.datetimepicker({
            lang: $datepicker.data('lang'),
            inline: true,
            timepicker: false,
            startDate: $datepicker.data('start-date'),
            formatDate: 'Y-m-d',
            onSelectDate: function (ct) {
                $('#deadline').val(ct.dateFormat('Y-m-d'));
            }
        });
        $('#clear-date').click(function () {
            $('#deadline').val('');
            return false;
        });
    }

    // Issues
    if ($('.repository.view.issue').length > 0) {
        // Edit issue title
        var $issue_title = $('#issue-title');
        var $edit_input = $('#edit-title-input input');
        var editTitleToggle = function () {
            $issue_title.toggle();
            $('.not-in-edit').toggle();
            $('#edit-title-input').toggle();
            $('.in-edit').toggle();
            $edit_input.focus();
            return false;
        };
        $('#edit-title').click(editTitleToggle);
        $('#cancel-edit-title').click(editTitleToggle);
        $('#save-edit-title').click(editTitleToggle).click(function () {
            if ($edit_input.val().length == 0 ||
                $edit_input.val() == $issue_title.text()) {
                $edit_input.val($issue_title.text());
                return false;
            }

            $.post($(this).data('update-url'), {
                    "_csrf": csrf,
                    "title": $edit_input.val()
                },
                function (data) {
                    $edit_input.val(data.title);
                    $issue_title.text(data.title);
                });
            return false;
        });

        // Edit issue or comment content
        $('.edit-content').click(function () {
            var $segment = $(this).parent().parent().next();
            var $edit_content_zone = $segment.find('.edit-content-zone');
            var $render_content = $segment.find('.render-content');
            var $raw_content = $segment.find('.raw-content');
            var $textarea;

            // Setup new form
            if ($edit_content_zone.html().length == 0) {
                $edit_content_zone.html($('#edit-content-form').html());
                $textarea = $segment.find('textarea');

                // Give new write/preview data-tab name to distinguish from others
                var $edit_content_form = $edit_content_zone.find('.ui.comment.form');
                var $tabular_menu = $edit_content_form.find('.tabular.menu');
                $tabular_menu.attr('data-write', $edit_content_zone.data('write'));
                $tabular_menu.attr('data-preview', $edit_content_zone.data('preview'));
                $tabular_menu.find('.write.item').attr('data-tab', $edit_content_zone.data('write'));
                $tabular_menu.find('.preview.item').attr('data-tab', $edit_content_zone.data('preview'));
                $edit_content_form.find('.write.segment').attr('data-tab', $edit_content_zone.data('write'));
                $edit_content_form.find('.preview.segment').attr('data-tab', $edit_content_zone.data('preview'));

                initCommentPreviewTab($edit_content_form);

                $edit_content_zone.find('.cancel.button').click(function () {
                    $render_content.show();
                    $edit_content_zone.hide();
                });
                $edit_content_zone.find('.save.button').click(function () {
                    $render_content.show();
                    $edit_content_zone.hide();

                    $.post($edit_content_zone.data('update-url'), {
                            "_csrf": csrf,
                            "content": $textarea.val(),
                            "context": $edit_content_zone.data('context')
                        },
                        function (data) {
                            if (data.length == 0) {
                                $render_content.html($('#no-content').html());
                            } else {
                                $render_content.html(data.content);
                                emojify.run($render_content[0]);
                                $('pre code', $render_content[0]).each(function (i, block) {
                                    hljs.highlightBlock(block);
                                });
                            }
                        });
                });
            } else {
                $textarea = $segment.find('textarea');
            }

            // Show write/preview tab and copy raw content as needed
            $edit_content_zone.show();
            $render_content.hide();
            if ($textarea.val().length == 0) {
                $textarea.val($raw_content.text());
            }
            $textarea.focus();
            return false;
        });

        // Change status
        var $status_btn = $('#status-button');
        $('#content').keyup(function () {
            if ($(this).val().length == 0) {
                $status_btn.text($status_btn.data('status'))
            } else {
                $status_btn.text($status_btn.data('status-and-comment'))
            }
        });
        $status_btn.click(function () {
            $('#status').val($status_btn.data('status-val'));
            $('#comment-form').submit();
        });
    }

    // Diff
    if ($('.repository.diff').length > 0) {
        var $counter = $('.diff-counter');
        if ($counter.length >= 1) {
            $counter.each(function (i, item) {
                var $item = $(item);
                var addLine = $item.find('span[data-line].add').data("line");
                var delLine = $item.find('span[data-line].del').data("line");
                var addPercent = parseFloat(addLine) / (parseFloat(addLine) + parseFloat(delLine)) * 100;
                $item.find(".bar .add").css("width", addPercent + "%");
            });
        }
    }

    // Quick start and repository home
    $('#repo-clone-ssh').click(function () {
        $('.clone-url').text($(this).data('link'));
        $('#repo-clone-url').val($(this).data('link'));
        $(this).addClass('blue');
        $('#repo-clone-https').removeClass('blue');
        localStorage.setItem('repo-clone-protocol', 'ssh');
    });
    $('#repo-clone-https').click(function () {
        $('.clone-url').text($(this).data('link'));
        $('#repo-clone-url').val($(this).data('link'));
        $(this).addClass('blue');
        $('#repo-clone-ssh').removeClass('blue');
        localStorage.setItem('repo-clone-protocol', 'https');
    });
    $('#repo-clone-url').click(function () {
        $(this).select();
    });

    // Pull request
    if ($('.repository.compare.pull').length > 0) {
        initFilterSearchDropdown('.choose.branch .dropdown');
    }
}

function initRepositoryCollaboration(){
    console.log('initRepositoryCollaboration');

// Change collaborator access mode
    $('.access-mode.menu .item').click(function(){
        var $menu = $(this).parent();
        $.post($menu.data('url'), {
            "_csrf": csrf,
            "uid": $menu.data('uid'),
            "mode": $(this).data('value')
        })
    });
}

function initWiki() {
    if ($('.repository.wiki').length == 0) {
        return;
    }


    if ($('.repository.wiki.new').length > 0) {
        var $edit_area = $('#edit-area');
        var simplemde = new SimpleMDE({
            autoDownloadFontAwesome: false,
            element: $edit_area[0],
            previewRender: function (plainText, preview) { // Async method
                setTimeout(function () {
                    // FIXME: still send render request when return back to edit mode
                    $.post($edit_area.data('url'), {
                            "_csrf": csrf,
                            "mode": "gfm",
                            "context": $edit_area.data('context'),
                            "text": plainText
                        },
                        function (data) {
                            preview.innerHTML = '<div class="markdown">' + data + '</div>';
                            emojify.run($('.editor-preview')[0]);
                        }
                    );
                }, 0);

                return "Loading...";
            },
            renderingConfig: {
                singleLineBreaks: false
            },
            spellChecker: false,
            tabSize: 4,
            toolbar: ["bold", "italic", "strikethrough", "|",
                "heading", "heading-1", "heading-2", "heading-3", "|",
                "code", "quote", "|",
                "unordered-list", "ordered-list", "|",
                "link", "image", "horizontal-rule", "|",
                "preview", "fullscreen"]
        })
    }
}

function initOrganization() {
    if ($('.organization').length == 0) {
        return;
    }

    // Options
    if ($('.organization.settings.options').length > 0) {
        $('#org_name').keyup(function () {
            var $prompt_span = $('#org-name-change-prompt');
            if ($(this).val().toString().toLowerCase() != $(this).data('org-name').toString().toLowerCase()) {
                $prompt_span.show();
            } else {
                $prompt_span.hide();
            }
        });
    }
}

function initUserSettings() {
    console.log('initUserSettings');

    // Options
    if ($('.user.settings.profile').length > 0) {
        $('#username').keyup(function () {
            var $prompt_span = $('#name-change-prompt');
            if ($(this).val().toString().toLowerCase() != $(this).data('name').toString().toLowerCase()) {
                $prompt_span.show();
            } else {
                $prompt_span.hide();
            }
        });
    }
}

function initWebhook() {
    if ($('.new.webhook').length == 0) {
        return;
    }

    $('.events.checkbox input').change(function () {
        if ($(this).is(':checked')) {
            $('.events.fields').show();
        }
    });
    $('.non-events.checkbox input').change(function () {
        if ($(this).is(':checked')) {
            $('.events.fields').hide();
        }
    });

    // Test delivery
    $('#test-delivery').click(function () {
        var $this = $(this);
        $this.addClass('loading disabled');
        $.post($this.data('link'), {
            "_csrf": csrf
        }).done(
            setTimeout(function () {
                window.location.href = $this.data('redirect');
            }, 5000)
        )
    });
}


function initAdmin() {
    if ($('.admin').length == 0) {
        return;
    }

    // New user
    if ($('.admin.new.user').length > 0 ||
        $('.admin.edit.user').length > 0) {
        $('#login_type').change(function () {
            if ($(this).val().substring(0, 1) == '0') {
                $('#login_name').removeAttr('required');
                $('.non-local').hide();
                $('.local').show();
                $('#user_name').focus();

                if ($(this).data('password') == "required") {
                    $('#password').attr('required', 'required');
                }

            } else {
                $('#login_name').attr('required', 'required');
                $('.non-local').show();
                $('.local').hide();
                $('#login_name').focus();

                $('#password').removeAttr('required');
            }
        });
    }


    // New authentication
    if ($('.admin.new.authentication').length > 0) {
        $('#auth_type').change(function () {
            $('.ldap').hide();
            $('.dldap').hide();
            $('.smtp').hide();
            $('.pam').hide();

            var auth_type = $(this).val();
            switch (auth_type) {
                case '2':     // LDAP
                    $('.ldap').show();
                    break;
                case '3':     // SMTP
                    $('.smtp').show();
                    break;
                case '4':     // PAM
                    $('.pam').show();
                    break;
                case '5':     // LDAP
                    $('.dldap').show();
                    break;
            }
        });
    }

    // Notice
    if ($('.admin.notice')) {
        var $detail_modal = $('#detail-modal');

        // Attach view detail modals
        $('.view-detail').click(function () {
            $detail_modal.find('.content p').text($(this).data('content'));
            $detail_modal.modal('show');
            return false;
        });

        // Select actions
        var $checkboxes = $('.select.table .ui.checkbox');
        $('.select.action').click(function () {
            switch ($(this).data('action')) {
                case 'select-all':
                    $checkboxes.checkbox('check');
                    break;
                case 'deselect-all':
                    $checkboxes.checkbox('uncheck');
                    break;
                case 'inverse':
                    $checkboxes.checkbox('toggle');
                    break;
            }
        });
        $('#delete-selection').click(function () {
            var $this = $(this);
            $this.addClass("loading disabled");
            var ids = [];
            $checkboxes.each(function () {
                if ($(this).checkbox('is checked')) {
                    ids.push($(this).data('id'));
                }
            });
            $.post($this.data('link'), {
                "_csrf": csrf,
                "ids": ids
            }).done(function () {
                window.location.href = $this.data('redirect');
            });
        });
    }
}

function buttonsClickOnEnter() {
    $('.ui.button').keypress(function (e) {
        if (e.keyCode == 13 || e.keyCode == 32) // enter key or space bar
            $(this).click();
    });
}

function hideWhenLostFocus(body, parent) {
    $(document).click(function (e) {
        var target = e.target;

        if (!$(target).is(body) && !$(target).parents().is(parent)) {
            $(body).hide();
        }
    });
}

function searchUsers() {
    if (!$('#search-user-box .results').length) {
        return;
    }

    var $search_user_box = $('#search-user-box');
    var $result_list = $search_user_box.find('.results');
    $search_user_box.keyup(function () {
        var $this = $(this);
        var keyword = $this.find('input').val();
        if (keyword.length < 2) {
            $result_list.hide();
            return;
        }

        $.ajax({
            url: suburl + '/api/v1/users/search?q=' + keyword,
            dataType: "json",
            success: function (response) {
                var notEmpty = function (str) {
                    return str && str.length > 0;
                };

                $result_list.html('');

                if (response.ok && response.data.length) {
                    var html = '';
                    $.each(response.data, function (i, item) {
                        html += '<div class="item"><img class="ui avatar image" src="' + item.avatar_url + '"><span class="username">' + item.username + '</span>';
                        if (notEmpty(item.full_name)) {
                            html += ' (' + item.full_name + ')';
                        }
                        html += '</div>';
                    });
                    $result_list.html(html);
                    $this.find('.results .item').click(function () {
                        $this.find('input').val($(this).find('.username').text());
                        $result_list.hide();
                    });
                    $result_list.show();
                } else {
                    $result_list.hide();
                }
            }
        });
    });
    $search_user_box.find('input').focus(function () {
        $search_user_box.keyup();
    });
    hideWhenLostFocus('#search-user-box .results', '#search-user-box');
}

// FIXME: merge common parts in two functions
function searchRepositories() {
    if (!$('#search-repo-box .results').length) {
        return;
    }

    var $search_repo_box = $('#search-repo-box');
    var $result_list = $search_repo_box.find('.results');
    $search_repo_box.keyup(function () {
        var $this = $(this);
        var keyword = $this.find('input').val();
        if (keyword.length < 2) {
            $result_list.hide();
            return;
        }

        $.ajax({
            url: suburl + '/api/v1/repos/search?q=' + keyword + "&uid=" + $search_repo_box.data('uid'),
            dataType: "json",
            success: function (response) {
                var notEmpty = function (str) {
                    return str && str.length > 0;
                };

                $result_list.html('');

                if (response.ok && response.data.length) {
                    var html = '';
                    $.each(response.data, function (i, item) {
                        html += '<div class="item"><i class="icon octicon octicon-repo"></i> <span class="fullname">' + item.full_name + '</span></div>';
                    });
                    $result_list.html(html);
                    $this.find('.results .item').click(function () {
                        $this.find('input').val($(this).find('.fullname').text().split("/")[1]);
                        $result_list.hide();
                    });
                    $result_list.show();
                } else {
                    $result_list.hide();
                }
            }
        });
    });
    $search_repo_box.find('input').focus(function () {
        $search_repo_box.keyup();
    });
    hideWhenLostFocus('#search-repo-box .results', '#search-repo-box');
}

$(document).ready(function () {
    csrf = $('meta[name=_csrf]').attr("content");
    suburl = $('meta[name=_suburl]').attr("content");

    // Show exact time
    $('.time-since').each(function () {
        $(this).addClass('poping up').attr('data-content', $(this).attr('title')).attr('data-variation', 'inverted tiny').attr('title', '');
    });

    // Semantic UI modules.
    $('.dropdown').dropdown();
    $('.jump.dropdown').dropdown({
        action: 'hide',
        onShow: function () {
            $('.poping.up').popup('hide');
        }
    });
    $('.slide.up.dropdown').dropdown({
        transition: 'slide up'
    });
    $('.upward.dropdown').dropdown({
        direction: 'upward'
    });
    $('.ui.accordion').accordion();
    $('.ui.checkbox').checkbox();
    $('.ui.progress').progress({
        showActivity: false
    });
    $('.poping.up').popup();
    $('.top.menu .poping.up').popup({
        onShow: function () {
            if ($('.top.menu .menu.transition').hasClass('visible')) {
                return false;
            }
        }
    });
    $('.tabular.menu .item').tab();
    $('.tabable.menu .item').tab();

    $('.toggle.button').click(function () {
        $($(this).data('target')).slideToggle(100);
    });

    // Highlight JS
    if (typeof hljs != 'undefined') {
        hljs.initHighlightingOnLoad();
    }

    // Dropzone
    if ($('#dropzone').length > 0) {
        // Disable auto discover for all elements:
        Dropzone.autoDiscover = false;

        var filenameDict = {};
        var $dropz = $('#dropzone');
        $dropz.dropzone({
            url: $dropz.data('upload-url'),
            headers: {"X-Csrf-Token": csrf},
            maxFiles: $dropz.data('max-file'),
            maxFilesize: $dropz.data('max-size'),
            acceptedFiles: ($dropz.data('accepts') === '*/*') ? null : $dropz.data('accepts'),
            addRemoveLinks: true,
            dictDefaultMessage: $dropz.data('default-message'),
            dictInvalidFileType: $dropz.data('invalid-input-type'),
            dictFileTooBig: $dropz.data('file-too-big'),
            dictRemoveFile: $dropz.data('remove-file'),
            init: function () {
                this.on("success", function (file, data) {
                    filenameDict[file.name] = data.uuid;
                    $('.attachments').append('<input id="' + data.uuid + '" name="attachments" type="hidden" value="' + data.uuid + '">');
                });
                this.on("removedfile", function (file) {
                    if (file.name in filenameDict) {
                        $('#' + filenameDict[file.name]).remove();
                    }
                })
            }
        });
    }

    // Emojify
    emojify.setConfig({
        img_dir: suburl + '/img/emoji',
        ignore_emoticons: true
    });
    var hasEmoji = document.getElementsByClassName('has-emoji');
    for (var i = 0; i < hasEmoji.length; i++) {
        emojify.run(hasEmoji[i]);
    }

    // Clipboard JS
    var clipboard = new Clipboard('.clipboard');
    clipboard.on('success', function (e) {
        e.clearSelection();

        $('#' + e.trigger.getAttribute('id')).popup('destroy');
        e.trigger.setAttribute('data-content', e.trigger.getAttribute('data-success'))
        $('#' + e.trigger.getAttribute('id')).popup('show');
        e.trigger.setAttribute('data-content', e.trigger.getAttribute('data-original'))
    });

    clipboard.on('error', function (e) {
        $('#' + e.trigger.getAttribute('id')).popup('destroy');
        e.trigger.setAttribute('data-content', e.trigger.getAttribute('data-error'))
        $('#' + e.trigger.getAttribute('id')).popup('show');
        e.trigger.setAttribute('data-content', e.trigger.getAttribute('data-original'))
    });

    // Helpers.
    $('.delete-button').click(function () {
        var $this = $(this);
        $('.delete.modal').modal({
            closable: false,
            onApprove: function () {
                if ($this.data('type') == "form") {
                    $($this.data('form')).submit();
                    return;
                }

                $.post($this.data('url'), {
                    "_csrf": csrf,
                    "id": $this.data("id")
                }).done(function (data) {
                    window.location.href = data.redirect;
                });
            }
        }).modal('show');
        return false;
    });
    $('.show-panel.button').click(function () {
        $($(this).data('panel')).show();
    });
    $('.show-modal.button').click(function () {
        $($(this).data('modal')).modal('show');
    });
    $('.delete-post.button').click(function(){
        var $this = $(this);
        $.post($this.data('request-url'),{
            "_csrf": csrf
        }).done(function(){
            window.location.href = $this.data('done-url');
        });
    });

    // Set anchor.
    $('.markdown').each(function () {
        var headers = {};
        $(this).find('h1, h2, h3, h4, h5, h6').each(function () {
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
    });

    buttonsClickOnEnter();
    searchUsers();
    searchRepositories();

    initCommentForm();
    initInstall();
    initRepository();
    initWiki();
    initOrganization();
    initWebhook();
    initAdmin();

    var routes = {
        'div.user.settings': initUserSettings,
        'div.repository.settings.collaboration': initRepositoryCollaboration
    };

    var selector;
    for (selector in routes) {
        if ($(selector).length > 0) {
            routes[selector]();
            break;
        }
    }
});

$(window).load(function () {
    function changeHash(hash) {
        if (history.pushState) {
            history.pushState(null, null, hash);
        }
        else {
            location.hash = hash;
        }
    }

    function deSelect() {
        if (window.getSelection) {
            window.getSelection().removeAllRanges();
        } else {
            document.selection.empty();
        }
    }

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
                changeHash('#L' + a + '-' + 'L' + b);
                return
            }
        }
        $select.addClass('active');
        changeHash('#' + $select.attr('rel'));
    }

    // Code view.
    if ($('.code-view .linenums').length > 0) {
        var $block = $('.code-view .linenums');
        var lines = $block.html().split("\n");
        $block.html('');

        var $num_list = $('.code-view .lines-num');

        // Building blocks.
        for (var i = 0; i < lines.length; i++) {
            $block.append('<li class="L' + (i + 1) + '" rel="L' + (i + 1) + '">' + lines[i] + '</li>');
            $num_list.append('<span id="L' + (i + 1) + '">' + (i + 1) + '</span>');
        }

        $(document).on('click', '.lines-num span', function (e) {
            var $select = $(this);
            var $list = $select.parent().siblings('.lines-code').find('ol.linenums > li');
            selectRange($list, $list.filter('[rel=' + $select.attr('id') + ']'), (e.shiftKey ? $list.filter('.active').eq(0) : null));
            deSelect();
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
    }

    // Repo clone url.
    if ($('#repo-clone-url').length > 0) {
        switch (localStorage.getItem('repo-clone-protocol')) {
            case 'ssh':
                if ($('#repo-clone-ssh').click().length === 0) {
                    $('#repo-clone-https').click();
                }
                ;
                break;
            default:
                $('#repo-clone-https').click();
                break;
        }
    }
});