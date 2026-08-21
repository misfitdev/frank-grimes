// Progressive enhancement. The page is complete without this file.
(function () {
  "use strict";

  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  document.documentElement.classList.add("js");

  // Lift the install commands off the page. Every code block here is a command
  // someone is meant to run, so each one gets a copy control.
  function addCopyControls() {
    if (!navigator.clipboard) return;

    document.querySelectorAll(".code-block").forEach(function (block) {
      var button = document.createElement("button");
      button.type = "button";
      button.className = "copy-button";
      button.textContent = "Copy";
      button.setAttribute("aria-label", "Copy command to clipboard");

      var status = document.createElement("span");
      status.className = "visually-hidden";
      status.setAttribute("role", "status");

      button.addEventListener("click", function () {
        navigator.clipboard.writeText(block.innerText.trim()).then(
          function () {
            button.textContent = "Copied";
            button.classList.add("is-copied");
            status.textContent = "Command copied to clipboard";
            window.setTimeout(function () {
              button.textContent = "Copy";
              button.classList.remove("is-copied");
              status.textContent = "";
            }, 2000);
          },
          function () {
            button.textContent = "Press Cmd-C";
            window.setTimeout(function () {
              button.textContent = "Copy";
            }, 2000);
          }
        );
      });

      block.appendChild(button);
      block.appendChild(status);
    });
  }

  // Mark the section being read, the way a finger holds a place in a binder.
  function trackReadingPosition() {
    var links = new Map();
    document.querySelectorAll(".top-nav a[href^='#']").forEach(function (link) {
      var section = document.querySelector(link.getAttribute("href"));
      if (section) links.set(section, link);
    });
    if (!links.size) return;

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          links.forEach(function (link) {
            link.classList.remove("is-current");
          });
          links.get(entry.target).classList.add("is-current");
        });
      },
      { rootMargin: "-20% 0px -70% 0px" }
    );

    links.forEach(function (_link, section) {
      observer.observe(section);
    });
  }

  // Finishing the report earns the stamp. It only lands once.
  function stampOnFinish() {
    var target = document.querySelector("[data-stamp-on-read]");
    if (!target) return;

    if (reduced) {
      target.style.opacity = "0.82";
      return;
    }

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          target.classList.add("is-stamped");
          observer.disconnect();
        });
      },
      { threshold: 0.6 }
    );

    observer.observe(target);
  }

  addCopyControls();
  trackReadingPosition();
  stampOnFinish();
})();
