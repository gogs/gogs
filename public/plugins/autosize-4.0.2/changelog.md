## Changelog

##### v.4.0.2 - 2018-04-30
* More specific detection of when to change overflow. Merges #361.

##### v.4.0.1 - 2018-03-23
* Minor refactor & updated build dependencies

##### v.4.0.0 - 2017-07-12
* Changed how Autosize determines the initial height. Fixes #336.

##### v.3.0.21 - 2017-05-19
* Fixed bug with overflow detection which degraded performance of textareas that exceed their max-width. Fixes #333.

##### v.3.0.20 - 2016-12-04
* Fixed minor bug where the `resized` event would not fire under specific conditions when changing the overflow.

##### v.3.0.19 - 2016-11-23
* Bubble dispatched events. Merged #319.

##### v.3.0.18 - 2016-10-26
* Fixed Firefox issue where calling dispatchEvent on a detached element throws an error.  Fixes #317.

##### v.3.0.17 - 2016-7-25
* Fixed Chromium issue where getComputedStyle pixel value did not exactly match the style pixel value.  Fixes #306.
* Removed undocumented argument, minor refactoring, more comments.

##### v.3.0.16 - 2016-7-13
* Fixed issue with overflowing parent elements. Fixes #298.

##### v.3.0.15 - 2016-1-26
* Used newer Event constructor, when available. Fixes #280.

##### v.3.0.14 - 2015-11-11
* Fixed memory leak on destroy. Merged #271, fixes #270.
* Fixed bug in old versions of Firefox (1-5), fixes #246.

##### v.3.0.13 - 2015-09-26
* Fixed scroll-bar jumpiness in iOS. Merged #261, fixes #207.
* Fixed reflowing of initial text in Chrome and Safari.

##### v.3.0.12 - 2015-09-14
* Merged changes were discarded when building new dist files.  Merged #255, Fixes #257 for real this time.

##### v.3.0.11 - 2015-09-14
* Fixed regression from 3.0.10 that caused an error with ES5 browsers.  Merged #255, Fixes #257.

##### v.3.0.10 - 2015-09-10
* Removed data attribute as a way of tracking which elements autosize has been assigned to. fixes #254, fixes #200.

##### v.3.0.9 - 2015-09-02
* Fixed issue with assigning autosize to detached nodes. Merged #253, Fixes #234.

##### v.3.0.8 - 2015-06-29
* Fixed the `autosize:resized` event not being triggered when the overflow changes. Fixes #244.

##### v.3.0.7 - 2015-06-29
* Fixed jumpy behavior in Windows 8.1 mobile. Fixes #239.

##### v.3.0.6 - 2015-05-19
* Renamed 'dest' folder to 'dist' to follow common conventions.

##### v.3.0.5 - 2015-05-18
* Do nothing in Node.js environment.

##### v.3.0.4 - 2015-05-05
* Added options object for indicating if the script should set the overflowX and overflowY.  The default behavior lets the script control the overflows, which will normalize the appearance between browsers.  Fixes #220.

##### v.3.0.3 - 2015-04-23
* Avoided adjusting the height for hidden textarea elements.  Fixes #155.

##### v.3.0.2 - 2015-04-23
* Reworked to respect max-height of any unit-type.  Fixes #191.

##### v.3.0.1 - 2015-04-23
* Fixed the destroy event so that it removes its own event handler. Fixes #218.

##### v.3.0.0 - 2015-04-15
* Added new methods for updating and destroying:

	* autosize.update(elements)
	* autosize.destroy(elements)

* Renamed custom events as to not use jQuery's custom events namespace:

	* autosize.resized renamed to autosize:resized
	* autosize.update renamed to autosize:update
	* autosize.destroy renamed to autosize:destroy

##### v.2.0.1 - 2015-04-15
* Version bump for NPM publishing purposes

##### v.2.0.0 - 2015-02-25

* Smaller, simplier code-base
* New API.  Example usage: `autosize(document.querySelectorAll(textarea));`
* Dropped jQuery dependency
* Dropped IE7-IE8 support
* Dropped optional parameters
* Closes #98, closes #106, closes #123, fixes #129, fixes #132, fixes #139, closes #140, closes #166, closes #168, closes #192, closes #193, closes #197