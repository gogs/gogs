// Fix landing page tab title: "Introduction - ..." → "Gogs - ..."
(function () {
  var old = "Introduction - Gogs";
  var fix = function () {
    if (document.title.startsWith(old)) {
      document.title = document.title.replace(old, "Gogs");
    }
  };
  new MutationObserver(fix).observe(
    document.querySelector("title") || document.head,
    { childList: true, subtree: true, characterData: true }
  );
  fix();
  setTimeout(fix, 100);
})();
