"use strict";

var suburl;

function initCommentPreviewTab($form) {
  var $tabMenu = $form.find(".tabular.menu");
  $tabMenu.find(".item").tab();
  $tabMenu
    .find('.item[data-tab="' + $tabMenu.data("preview") + '"]')
    .click(function() {
      var $this = $(this);
      $.post(
        $this.data("url"),
        {
          mode: "gfm",
          context: $this.data("context"),
          text: $form
            .find(
              '.tab.segment[data-tab="' + $tabMenu.data("write") + '"] textarea'
            )
            .val()
        },
        function(data) {
          var $previewPanel = $form.find(
            '.tab.segment[data-tab="' + $tabMenu.data("preview") + '"]'
          );
          $previewPanel.html(data);
          emojify.run($previewPanel[0]);
          $("pre code", $previewPanel[0]).each(function(i, block) {
            hljs.highlightBlock(block);
          });
        }
      );
    });

  buttonsClickOnEnter();
}

function initCommentForm() {
  if ($(".comment.form").length == 0) {
    return;
  }

  initCommentPreviewTab($(".comment.form"));

  // Labels
  var $list = $(".ui.labels.list");
  var $noSelect = $list.find(".no-select");
  var $labelMenu = $(".select-label .menu");
  var hasLabelUpdateAction = $labelMenu.data("action") == "update";

  function updateIssueMeta(url, action, id) {
    $.post(url, {
      action: action,
      id: id
    });
  }

  // Add &nbsp; to each unselected label to keep UI looks good.
  // This should be added directly to HTML but somehow just get empty <span> on this page.
  $labelMenu
    .find(".item:not(.no-select) .octicon:not(.octicon-check)")
    .each(function() {
      $(this).html("&nbsp;");
    });
  $labelMenu.find(".item:not(.no-select)").click(function() {
    if ($(this).hasClass("checked")) {
      $(this).removeClass("checked");
      $(this)
        .find(".octicon")
        .removeClass("octicon-check")
        .html("&nbsp;");
      if (hasLabelUpdateAction) {
        updateIssueMeta(
          $labelMenu.data("update-url"),
          "detach",
          $(this).data("id")
        );
      }
    } else {
      $(this).addClass("checked");
      $(this)
        .find(".octicon")
        .addClass("octicon-check")
        .html("");
      if (hasLabelUpdateAction) {
        updateIssueMeta(
          $labelMenu.data("update-url"),
          "attach",
          $(this).data("id")
        );
      }
    }

    var labelIds = "";
    $(this)
      .parent()
      .find(".item")
      .each(function() {
        if ($(this).hasClass("checked")) {
          labelIds += $(this).data("id") + ",";
          $($(this).data("id-selector")).removeClass("hide");
        } else {
          $($(this).data("id-selector")).addClass("hide");
        }
      });
    if (labelIds.length == 0) {
      $noSelect.removeClass("hide");
    } else {
      $noSelect.addClass("hide");
    }
    $(
      $(this)
        .parent()
        .data("id")
    ).val(labelIds);
    return false;
  });
  $labelMenu.find(".no-select.item").click(function() {
    if (hasLabelUpdateAction) {
      updateIssueMeta($labelMenu.data("update-url"), "clear", "");
    }

    $(this)
      .parent()
      .find(".item")
      .each(function() {
        $(this).removeClass("checked");
        $(this)
          .find(".octicon")
          .removeClass("octicon-check")
          .html("&nbsp;");
      });

    $list.find(".item").each(function() {
      $(this).addClass("hide");
    });
    $noSelect.removeClass("hide");
    $(
      $(this)
        .parent()
        .data("id")
    ).val("");
  });

  function selectItem(select_id, input_id) {
    var $menu = $(select_id + " .menu");
    var $list = $(".ui" + select_id + ".list");
    var hasUpdateAction = $menu.data("action") == "update";

    $menu.find(".item:not(.no-select)").click(function() {
      $(this)
        .parent()
        .find(".item")
        .each(function() {
          $(this).removeClass("selected active");
        });

      $(this).addClass("selected active");
      if (hasUpdateAction) {
        updateIssueMeta($menu.data("update-url"), "", $(this).data("id"));
      }
      switch (input_id) {
        case "#milestone_id":
          var $milestoneAnchor = $('<a class="item"></a>');
          $milestoneAnchor.attr("href", $(this).data("href"));
          $milestoneAnchor.text($(this).text());
          $list.find(".selected").empty().append($milestoneAnchor);
          break;
        case "#assignee_id":
          var $assigneeAnchor = $('<a class="item"></a>');
          $assigneeAnchor.attr("href", $(this).data("href"));
          $assigneeAnchor.append(
            $('<img class="ui avatar image">').attr("src", $(this).data("avatar"))
          );
          $assigneeAnchor.append($("<span></span>").text($(this).text()));
          $list.find(".selected").empty().append($assigneeAnchor);
      }
      $(".ui" + select_id + ".list .no-select").addClass("hide");
      $(input_id).val($(this).data("id"));
    });
    $menu.find(".no-select.item").click(function() {
      $(this)
        .parent()
        .find(".item:not(.no-select)")
        .each(function() {
          $(this).removeClass("selected active");
        });

      if (hasUpdateAction) {
        updateIssueMeta($menu.data("update-url"), "", "");
      }

      $list.find(".selected").html("");
      $list.find(".no-select").removeClass("hide");
      $(input_id).val("");
    });
  }

  // Milestone and assignee
  selectItem(".select-milestone", "#milestone_id");
  selectItem(".select-assignee", "#assignee_id");
}

function initRepository() {
  if ($(".repository").length == 0) {
    return;
  }

  function initFilterSearchDropdown(selector) {
    var $dropdown = $(selector);
    $dropdown.dropdown({
      fullTextSearch: true,
      onChange: function(text, value, $choice) {
        window.location.href = $choice.data("url");
        console.log($choice.data("url"));
      },
      message: { noResults: $dropdown.data("no-results") }
    });
  }

  // File list and commits
  if (
    $(".repository.file.list").length > 0 ||
    ".repository.commits".length > 0
  ) {
    initFilterSearchDropdown(".choose.reference .dropdown");

    $(".reference.column").click(function() {
      $(".choose.reference .scrolling.menu").css("display", "none");
      $(".choose.reference .text").removeClass("black");
      $($(this).data("target")).css("display", "block");
      $(this)
        .find(".text")
        .addClass("black");
      return false;
    });
  }

  // Issues
  if ($(".repository.view.issue").length > 0) {
    // Edit issue title
    var $issueTitle = $("#issue-title");
    var $editInput = $("#edit-title-input").find("input");
    var editTitleToggle = function() {
      $issueTitle.toggle();
      $(".not-in-edit").toggle();
      $("#edit-title-input").toggle();
      $(".in-edit").toggle();
      $editInput.focus();
      return false;
    };
    $("#edit-title").click(editTitleToggle);
    $("#cancel-edit-title").click(editTitleToggle);
    $("#save-edit-title")
      .click(editTitleToggle)
      .click(function() {
        if (
          $editInput.val().length == 0 ||
          $editInput.val() == $issueTitle.text()
        ) {
          $editInput.val($issueTitle.text());
          return false;
        }

        $.post(
          $(this).data("update-url"),
          {
            title: $editInput.val()
          },
          function(data) {
            $editInput.val(data.title);
            $issueTitle.text(data.title);
          }
        );
        return false;
      });

    // Edit issue or comment content
    $(".edit-content").click(function() {
      var $segment = $(this)
        .parent()
        .parent()
        .parent()
        .next();
      var $editContentZone = $segment.find(".edit-content-zone");
      var $renderContent = $segment.find(".render-content");
      var $rawContent = $segment.find(".raw-content");
      var $textarea;

      // Setup new form
      if ($editContentZone.html().length == 0) {
        $editContentZone.html($("#edit-content-form").html());
        $textarea = $segment.find("textarea");

        // Give new write/preview data-tab name to distinguish from others
        var $editContentForm = $editContentZone.find(".ui.comment.form");
        var $tabMenu = $editContentForm.find(".tabular.menu");
        $tabMenu.attr("data-write", $editContentZone.data("write"));
        $tabMenu.attr("data-preview", $editContentZone.data("preview"));
        $tabMenu
          .find(".write.item")
          .attr("data-tab", $editContentZone.data("write"));
        $tabMenu
          .find(".preview.item")
          .attr("data-tab", $editContentZone.data("preview"));
        $editContentForm
          .find(".write.segment")
          .attr("data-tab", $editContentZone.data("write"));
        $editContentForm
          .find(".preview.segment")
          .attr("data-tab", $editContentZone.data("preview"));

        initCommentPreviewTab($editContentForm);

        $editContentZone.find(".cancel.button").click(function() {
          $renderContent.show();
          $editContentZone.hide();
        });
        $editContentZone.find(".save.button").click(function() {
          $renderContent.show();
          $editContentZone.hide();

          $.post(
            $editContentZone.data("update-url"),
            {
              content: $textarea.val(),
              context: $editContentZone.data("context")
            },
            function(data) {
              if (data.length == 0) {
                $renderContent.html($("#no-content").html());
              } else {
                $renderContent.html(data.content);
                emojify.run($renderContent[0]);
                $("pre code", $renderContent[0]).each(function(i, block) {
                  hljs.highlightBlock(block);
                });
              }
            }
          );
        });
      } else {
        $textarea = $segment.find("textarea");
      }

      // Show write/preview tab and copy raw content as needed
      $editContentZone.show();
      $renderContent.hide();
      if ($textarea.val().length == 0) {
        $textarea.val($rawContent.text());
      }
      $textarea.focus();
      return false;
    });

    // Delete comment
    $(".delete-comment").click(function() {
      var $this = $(this);
      if (confirm($this.data("locale"))) {
        $.post($this.data("url"), {
        }).done(function() {
          $("#" + $this.data("comment-id")).remove();
        });
      }
      return false;
    });

    // Change status
    var $statusButton = $("#status-button");
    $("#comment-form .edit_area").keyup(function() {
      if ($(this).val().length == 0) {
        $statusButton.text($statusButton.data("status"));
      } else {
        $statusButton.text($statusButton.data("status-and-comment"));
      }
    });
    $statusButton.click(function() {
      $("#status").val($statusButton.data("status-val"));
      $("#comment-form").submit();
    });
  }

  // Diff
  if ($(".repository.diff").length > 0) {
    var $counter = $(".diff-counter");
    if ($counter.length >= 1) {
      $counter.each(function(i, item) {
        var $item = $(item);
        var addLine = $item.find("span[data-line].add").data("line");
        var delLine = $item.find("span[data-line].del").data("line");
        var addPercent =
          (parseFloat(addLine) / (parseFloat(addLine) + parseFloat(delLine))) *
          100;
        $item.find(".bar .add").css("width", addPercent + "%");
      });
    }

    $(".diff-file-box .lines-num").click(function() {
      if ($(this).attr("id")) {
        window.location.href = "#" + $(this).attr("id");
      }
    });

    $(window)
      .on("hashchange", function(e) {
        $(".diff-file-box .lines-code.active").removeClass("active");
        var m = window.location.hash.match(/^#diff-.+$/);
        if (m) {
          $(m[0])
            .siblings(".lines-code")
            .addClass("active");
        }
      })
      .trigger("hashchange");
  }

  // Quick start and repository home
  $("#repo-clone-ssh").click(function() {
    $(".clone-url").text($(this).data("link"));
    $("#repo-clone-url").val($(this).data("link"));
    $(this).addClass("blue");
    $("#repo-clone-https").removeClass("blue");
    localStorage.setItem("repo-clone-protocol", "ssh");
  });
  $("#repo-clone-https").click(function() {
    $(".clone-url").text($(this).data("link"));
    $("#repo-clone-url").val($(this).data("link"));
    $(this).addClass("blue");
    $("#repo-clone-ssh").removeClass("blue");
    localStorage.setItem("repo-clone-protocol", "https");
  });
  $("#repo-clone-url").click(function() {
    $(this).select();
  });

  // Pull request
  if ($(".repository.compare.pull").length > 0) {
    initFilterSearchDropdown(".choose.branch .dropdown");
  }
  if ($(".repository.view.pull").length > 0) {
    $(".comment.merge.box input[name=merge_style]").change(function() {
      if ($(this).val() === "create_merge_commit") {
        $(".commit.description.field").show();
      } else {
        $(".commit.description.field").hide();
      }
    });
  }
}

// For IE
String.prototype.endsWith = function(pattern) {
  var d = this.length - pattern.length;
  return d >= 0 && this.lastIndexOf(pattern) === d;
};

function buttonsClickOnEnter() {
  $(".ui.button").keypress(function(e) {
    if (e.keyCode == 13 || e.keyCode == 32)
      // enter key or space bar
      $(this).click();
  });
}

function hideWhenLostFocus(body, parent) {
  $(document).click(function(e) {
    var target = e.target;
    if (
      !$(target).is(body) &&
      !$(target)
        .parents()
        .is(parent)
    ) {
      $(body).hide();
    }
  });
}

// FIXME: merge common parts in two functions
function searchRepositories() {
  if (!$("#search-repo-box .results").length) {
    return;
  }

  var $searchRepoBox = $("#search-repo-box");
  var $results = $searchRepoBox.find(".results");
  $searchRepoBox.keyup(function() {
    var $this = $(this);
    var keyword = $this.find("input").val();
    if (keyword.length < 2) {
      $results.hide();
      return;
    }

    $.ajax({
      url:
        suburl +
        "/api/v1/repos/search?q=" +
        keyword +
        "&uid=" +
        $searchRepoBox.data("uid"),
      dataType: "json",
      success: function(response) {
        var notEmpty = function(str) {
          return str && str.length > 0;
        };

        $results.html("");

        if (response.ok && response.data.length) {
          var html = "";
          $.each(response.data, function(i, item) {
            html +=
              '<div class="item"><i class="octicon octicon-repo"></i> <span class="fullname">' +
              item.full_name +
              "</span></div>";
          });
          $results.html(html);
          $this.find(".results .item").click(function() {
            $this.find("input").val(
              $(this)
                .find(".fullname")
                .text()
                .split("/")[1]
            );
            $results.hide();
          });
          $results.show();
        } else {
          $results.hide();
        }
      }
    });
  });
  $searchRepoBox.find("input").focus(function() {
    $searchRepoBox.keyup();
  });
  hideWhenLostFocus("#search-repo-box .results", "#search-repo-box");
}

function initCodeView() {
  if ($(".code-view .linenums").length > 0) {
    $(document).on("click", ".lines-num span", function(e) {
      var $select = $(this);
      var $list = $select
        .parent()
        .siblings(".lines-code")
        .find("ol.linenums > li");
      selectRange(
        $list,
        $list.filter("[rel=" + $select.attr("id") + "]"),
        e.shiftKey ? $list.filter(".active").eq(0) : null
      );
      deSelect();
    });

    $(window)
      .on("hashchange", function(e) {
        var m = window.location.hash.match(/^#(L\d+)\-(L\d+)$/);
        var $list = $(".code-view ol.linenums > li");
        var $first;
        if (m) {
          $first = $list.filter("." + m[1]);
          selectRange($list, $first, $list.filter("." + m[2]));
          $("html, body").scrollTop($first.offset().top - 200);
          return;
        }
        m = window.location.hash.match(/^#(L\d+)$/);
        if (m) {
          $first = $list.filter("." + m[1]);
          selectRange($list, $first);
          $("html, body").scrollTop($first.offset().top - 200);
        }
      })
      .trigger("hashchange");
  }
}

$(document).ready(function() {
  suburl = $("meta[name=_suburl]").attr("content");

  // Set cursor to the end of autofocus input string
  $("input[autofocus]").each(function() {
    $(this).val($(this).val());
  });

  // Show exact time
  $(".time-since").each(function() {
    $(this)
      .addClass("poping up")
      .attr("data-content", $(this).attr("title"))
      .attr("data-variation", "inverted tiny")
      .attr("title", "");
  });

  // Semantic UI modules.
  $(".ui.dropdown").dropdown({
    forceSelection: false
  });
  $(".jump.dropdown").dropdown({
    action: "select",
    onShow: function() {
      $(".poping.up").popup("hide");
    }
  });
  $(".slide.up.dropdown").dropdown({
    transition: "slide up"
  });
  $(".upward.dropdown").dropdown({
    direction: "upward"
  });
  $(".ui.accordion").accordion();
  $(".ui.checkbox").checkbox();
  $(".ui.progress").progress({
    showActivity: false
  });
  $(".poping.up").popup();
  $(".top.menu .poping.up").popup({
    onShow: function() {
      if ($(".top.menu .menu.transition").hasClass("visible")) {
        return false;
      }
    }
  });
  $(".tabular.menu .item").tab();
  $(".tabable.menu .item").tab();

  $(".toggle.button").click(function() {
    $($(this).data("target")).slideToggle(100);
  });

  // Dropzone
  var $dropzone = $("#dropzone");
  if ($dropzone.length > 0) {
    var filenameDict = {};
    $dropzone.dropzone({
      url: $dropzone.data("upload-url"),
      maxFiles: $dropzone.data("max-file"),
      maxFilesize: $dropzone.data("max-size"),
      timeout: 0,
      acceptedFiles:
        $dropzone.data("accepts") === "*/*" ? null : $dropzone.data("accepts"),
      addRemoveLinks: true,
      dictDefaultMessage: $dropzone.data("default-message"),
      dictInvalidFileType: $dropzone.data("invalid-input-type"),
      dictFileTooBig: $dropzone.data("file-too-big"),
      dictRemoveFile: $dropzone.data("remove-file"),
      init: function() {
        this.on("success", function(file, data) {
          filenameDict[file.name] = data.uuid;
          var input = $(
            '<input id="' + data.uuid + '" name="files" type="hidden">'
          ).val(data.uuid);
          $(".files").append(input);
        });
        this.on("removedfile", function(file) {
          if (file.name in filenameDict) {
            $("#" + filenameDict[file.name]).remove();
          }
          if ($dropzone.data("remove-url")) {
            $.post($dropzone.data("remove-url"), {
              file: filenameDict[file.name]
            });
          }
        });
      }
    });
  }

  // Emojify
  emojify.setConfig({
    img_dir: suburl + "/img/emoji",
    ignore_emoticons: true
  });
  var hasEmoji = document.getElementsByClassName("has-emoji");
  for (var i = 0; i < hasEmoji.length; i++) {
    emojify.run(hasEmoji[i]);
  }

  // Clipboard JS
  var clipboard = new ClipboardJS(".clipboard");
  clipboard.on("success", function(e) {
    e.clearSelection();

    $("#" + e.trigger.getAttribute("id")).popup("destroy");
    e.trigger.setAttribute(
      "data-content",
      e.trigger.getAttribute("data-success")
    );
    $("#" + e.trigger.getAttribute("id")).popup("show");
    e.trigger.setAttribute(
      "data-content",
      e.trigger.getAttribute("data-original")
    );
  });

  clipboard.on("error", function(e) {
    $("#" + e.trigger.getAttribute("id")).popup("destroy");
    e.trigger.setAttribute(
      "data-content",
      e.trigger.getAttribute("data-error")
    );
    $("#" + e.trigger.getAttribute("id")).popup("show");
    e.trigger.setAttribute(
      "data-content",
      e.trigger.getAttribute("data-original")
    );
  });

  // Helpers
  $(".delete-button").click(function() {
    var $this = $(this);
    $(".delete.modal")
      .modal({
        closable: false,
        onApprove: function() {
          if ($this.data("type") == "form") {
            $($this.data("form")).submit();
            return;
          }

          $.post($this.data("url"), {
            id: $this.data("id")
          }).done(function(data) {
            window.location.href = data.redirect;
          });
        }
      })
      .modal("show");
    return false;
  });
  $(".show-modal.button").click(function() {
    $($(this).data("modal")).modal("show");
  });
  // To make arbitrary form element to behave like a submit button
  $(".submit-button").click(function() {
    $($(this).data("form")).submit();
  });

  // Intercept the legacy sign-out form so it talks to the JSON API and
  // navigates to the URL the server hands back (honors CustomLogoutURL).
  $("#logout-form").on("submit", function(event) {
    event.preventDefault();
    var $form = $(this);
    var doneUrl = $form.data("done-url");
    $.ajax({
      url: $form.attr("action"),
      method: "POST"
    }).done(function(data) {
      var target = (data && data.redirectTo) ? data.redirectTo : doneUrl;
      window.location.assign(target);
    }).fail(function() {
      window.location.assign(doneUrl);
    });
  });

  // Set anchor.
  $(".markdown").each(function() {
    var headers = {};
    $(this)
      .find("h1, h2, h3, h4, h5, h6")
      .each(function() {
        var node = $(this);
        // Rendered Markdown headings have an id like "user-content-setup", as
        // on GitHub. The wrapper gets "setup", so links to "#setup" work.
        var id = node.attr("id");
        if (id && id.indexOf("user-content-") === 0) {
          var slug = id.slice("user-content-".length);
          var wrap = document.createElement("div");
          wrap.id = slug;
          wrap.className = "anchor-wrap";
          node.wrap(wrap);
          var anchor = document.createElement("a");
          anchor.className = "anchor";
          anchor.href = "#" + encodeURIComponent(slug);
          anchor.setAttribute("aria-label", "Link to this section");
          anchor.innerHTML = '<span class="octicon octicon-link"></span>';
          node.append(anchor);
          return;
        }
        var val = encodeURIComponent(
          node
            .text()
            .toLowerCase()
            .replace(/[^\u00C0-\u1FFF\u2C00-\uD7FF\w\- ]/g, "")
            .replace(/[ ]/g, "-")
        );
        var name = val;
        if (headers[val] > 0) {
          name = val + "-" + headers[val];
        }
        if (headers[val] == undefined) {
          headers[val] = 1;
        } else {
          headers[val] += 1;
        }
        node = node.wrap('<div id="' + name + '" class="anchor-wrap" ></div>');
        node.append(
          '<a class="anchor" href="#' +
            name +
            '"><span class="octicon octicon-link"></span></a>'
        );
      });
  });

  buttonsClickOnEnter();
  searchRepositories();

  initCommentForm();
  initRepository();
  initCodeView();

  // Repo clone url.
  if ($("#repo-clone-url").length > 0) {
    switch (localStorage.getItem("repo-clone-protocol")) {
      case "ssh":
        if ($("#repo-clone-ssh").click().length === 0) {
          $("#repo-clone-https").click();
        }
        break;
      default:
        $("#repo-clone-https").click();
        break;
    }
  }
});

function changeHash(hash) {
  if (history.pushState) {
    history.pushState(null, null, hash);
  } else {
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
  $list.removeClass("active");
  if ($from) {
    var a = parseInt($select.attr("rel").substr(1));
    var b = parseInt($from.attr("rel").substr(1));
    var c;
    if (a != b) {
      if (a > b) {
        c = a;
        a = b;
        b = c;
      }
      var classes = [];
      for (var i = a; i <= b; i++) {
        classes.push(".L" + i);
      }
      $list.filter(classes.join(",")).addClass("active");
      changeHash("#L" + a + "-" + "L" + b);
      return;
    }
  }
  $select.addClass("active");
  changeHash("#" + $select.attr("rel"));
}

$(function() {
  if ($(".user.signin").length > 0) return;
  $("form").areYouSure();
});
