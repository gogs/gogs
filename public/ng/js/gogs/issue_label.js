// when dom ready, init issue label events
$(document).ready(function(){
    var labelColors = ["#e11d21","#EB6420","#FBCA04","#009800",
    "#006B75","#207DE5","#0052cc","#53E917",
    "#F6C6C7","#FAD8C7","#FEF2C0","#BFE5BF",
    "#BFDADC","#C7DEF8","#BFD4F2","#D4C5F9"];

    var colorDropHtml = "";
    labelColors.forEach(function(item){
       colorDropHtml += '<a class="color" style="background-color:'+item+'" data-color-hex="'+item+'"></a>';
    });



    // render label color input
    var color_input = $('#label-add-color');
    var color_label = $('#label-add-form .label-color-drop label');
    color_label.css("background-color",labelColors[0]);
    color_input.val(labelColors[0]);


    // render label color drop
    function render_color_drop($e){
        $e.find('.label-color-drop .drop-down')
            .html(colorDropHtml)
            .on("click","a",function(){
                var $form = $(this).parents(".form");
                var color_label = $form.find(".label-color-drop label");
                var color_input = $form.find("input[name=color]");
                var color = $(this).data("color-hex");
                color_label.css("background-color",color);
                color_input.val(color);
            });
    }


    //  color drop visible
    var form = $('#label-add-form');
    render_color_drop(form);
    $('#label-new-btn').on("click",function(){
        if(form.hasClass("hidden")){
            form.removeClass("hidden");
        }
    });
    $('#label-cancel-btn').on("click",function(){
        form.addClass("hidden");
    });

    // label edit form render
    var $edit_form_tpl = $("#label-edit-form-tpl");
    $("#label-list").on("click","a.edit",function(){
        var $label_item = $(this).parents(".item");
        var $clone_form = $edit_form_tpl.clone();
        render_color_drop($clone_form);

        // add default color
        var color_label = $clone_form.find(".label-color-drop label");
        var color_input = $clone_form.find("input[name=color]");
        var color = $label_item.find(".label").data("color-hex");
        color_label.css("background-color",color);
        color_input.val(color);

        // add label name
        $clone_form.find("input[name=name]").val($label_item.find(".label").text());

        // add label id
        $clone_form.find("input[name=id]").val($label_item.attr("id").replace("label-",""));

        // append form
        $label_item.after($clone_form.show());

        // add cancel button event
        $('#label-edit-cancel-btn').on("click",function(){
           $clone_form.remove();
        });

    });

    // label delete form render
    var $del_form_tpl = $('#label-delete-form-tpl');
    $("#label-list").on("click","a.delete",function(){
        var $label_item = $(this).parents(".item");
        var $clone_form = $del_form_tpl.clone();

        // add label id
        $clone_form.find("input[name=id]").val($label_item.attr("id").replace("label-",""));

        // append form
        $label_item.after($clone_form.show());

        // add cancel button event
        $('#label-del-cancel-btn').on("click",function(){
            $clone_form.remove();
        });
    });

});
