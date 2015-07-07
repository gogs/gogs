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

$(document).ready(function () {
    // Semantic UI modules.
    $('.dropdown').dropdown();
    $('.slide.up.dropdown').dropdown({
        transition: 'slide up'
    });

    initInstall();
});