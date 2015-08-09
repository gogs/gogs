'use strict';

var csrf;

function initInstall() {
    if ($('.install').length == 0) {
        return;
    }

    // Database type change detection.
    $("#db_type").change(function () {
        var db_type = $('#db_type').val();
        if (db_type === "SQLite3") {
            $('#sql_settings').hide();
            $('#pgsql_settings').hide();
            $('#sqlite_settings').show();
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
};

function initRepository() {
    if ($('.repository').length == 0) {
        return;
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
            var color_hex = $(this).data('color-hex')
            $('.color-picker').val(color_hex);
            $('.minicolors-swatch-color').css("background-color", color_hex);
        });
        $('.edit-label-button').click(function () {
            $('#label-modal-id').val($(this).data('id'));
            $('.edit-label .new-label-input').val($(this).data('title'));
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
        var $datepicker = $('.milestone.datepicker')
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

    // Settings
    if ($('.repository.settings').length > 0) {
        $('#add-deploy-key').click(function () {
            $('#add-deploy-key-panel').show();
        });
    }

    // Pull request
    if ($('.repository.compare.pull').length > 0) {
        var $branch_dropdown = $('.choose.branch .dropdown')
        $branch_dropdown.dropdown({
            fullTextSearch: true,
            onChange: function (text, value, $choice) {
                window.location.href = $choice.data('url');
            },
            message: {noResults: $branch_dropdown.data('no-results')}
        });
    }
};

$(document).ready(function () {
    csrf = $('meta[name=_csrf]').attr("content");

    // Semantic UI modules.
    $('.dropdown').dropdown();
    $('.jump.dropdown').dropdown({
        action: 'hide'
    });
    $('.slide.up.dropdown').dropdown({
        transition: 'slide up'
    });
    $('.ui.accordion').accordion();
    $('.ui.checkbox').checkbox();
    $('.ui.progress').progress({
        showActivity: false
    });
    $('.poping.up').popup();

    // Comment form
    $('.comment.form .tabular.menu .item').tab();
    $('.comment.form .tabular.menu .item[data-tab="preview"]').click(function () {
        var $this = $(this);
        console.log($('.comment.form .tab.segment[data-tab="write"] textarea').val())
        console.log($('.comment.form .tab.segment[data-tab="preview"]').html())
        $.post($this.data('url'), {
                "_csrf": csrf,
                "mode": "gfm",
                "context": $this.data('context'),
                "text": $('.comment.form .tab.segment[data-tab="write"] textarea').val()
            },
            function (data) {
                console.log(data)
                $('.comment.form .tab.segment[data-tab="preview"]').html(data);
            }
        )
        ;
    })

    // Helpers.
    $('.delete-button').click(function () {
        var $this = $(this);
        $('.delete.modal').modal({
            closable: false,
            onApprove: function () {
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

    initInstall();
    initRepository();
});