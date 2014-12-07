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
    var color_label = $('#label-color-drop label');
    color_label.css("background-color",labelColors[0]);
    color_input.val(labelColors[0]);


    // render label color drop
    $('#label-color-drop .drop-down')
        .html(colorDropHtml)
        .on("click","a",function(){
            var color = $(this).data("color-hex");
            color_label.css("background-color",color);
            color_input.val(color);
        });

    //  color drop visible
    var form = $('#label-add-form');
    $('#label-new-btn').on("click",function(){
        if(form.hasClass("hidden")){
            form.removeClass("hidden");
        }
    });
    $('#label-cancel-btn').on("click",function(){
        form.addClass("hidden");
    })


});
