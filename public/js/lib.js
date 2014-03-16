/* ===========================================================
 * forked from bootstrap-confirmation.js
 * http://ethaizone.github.io/Bootstrap-Confirmation/
 * ===========================================================
 * Copyright 2013 Nimit Suwannagate <ethaizone@hotmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 * =========================================================== */
!function ($) {
	'use strict';

	//var for check event at body can have only one.
	var event_body = false;

	// CONFIRMATION PUBLIC CLASS DEFINITION
	// ===============================
	var Confirmation = function (element, options) {
		var that = this;

		this.init('confirmation', element, options);


		$(element).on('show.bs.confirmation', function(e) {
			that.options.onShow(e, this);

			$(this).addClass('open');

			var options = that.options;
			var all = options.all_selector;

			if(options.singleton) {
				$(all+'.in').not(that.$element).confirmation('hide');
			}
		});

		$(element).on('hide.bs.confirmation', function(e) {
			that.options.onHide(e, this);

			$(this).removeClass('open');
		});

		$(element).on('shown.bs.confirmation', function(e) {
			var options = that.options;
			var all = options.all_selector;

			that.$element.on('click.dismiss.bs.confirmation', '[data-dismiss="confirmation"]', $.proxy(that.hide, that));

			if(that.isPopout()) {
				if(!event_body) {
					event_body = $('body').on('click', function (e) {
						if(that.$element.is(e.target)) return;
						if(that.$element.has(e.target).length) return;
						if($('.popover').has(e.target).length) return;

						that.$element.confirmation('hide');

						$('body').unbind(e);

						event_body = false;

						return;
					});
				}
			}
		});

		$(element).on('click', function(e) {
			e.preventDefault();
		});
	}

	if (!$.fn.popover || !$.fn.tooltip) throw new Error('Confirmation requires popover.js and tooltip.js');

	Confirmation.DEFAULTS = $.extend({}, $.fn.popover.Constructor.DEFAULTS, {
		placement 		: 'top',
		title 			: 'Are you sure?',
		btnOkClass 		: 'btn btn-danger btn-sm',
		btnOkLabel 		: 'Yes',
		btnOkIcon 		: '',
		btnCancelClass 	: 'btn btn-default btn-sm',
		btnCancelLabel 	: 'Cancel',
		btnCancelIcon 	: '',
		href 			: '#',
		target 			: '_self',
		singleton 		: true,
		popout 			: true,
		onShow 			: function(event, element){},
		onHide 			: function(event, element){},
		onConfirm 		: function(event, element){},
		onCancel 		: function(event, element){},
		template 		:   '<div class="popover"><div class="arrow"></div>'
							+ '<h3 class="popover-title"></h3>'
							+ '<div class="popover-content">'
							+ '<div class="btn-group"><a data-apply="confirmation">Yes</a>'
							+ ' <a data-dismiss="confirmation">No</a></div>'
							+ '</div>'
							+ '</div>'
	});


	// NOTE: CONFIRMATION EXTENDS popover.js
	// ================================
	Confirmation.prototype = $.extend({}, $.fn.popover.Constructor.prototype);

	Confirmation.prototype.constructor = Confirmation;

	Confirmation.prototype.getDefaults = function () {
		return Confirmation.DEFAULTS;
	}

	Confirmation.prototype.setContent = function () {
		var that = this;
		var $tip    = this.tip();
		var title   = this.getTitle();
		var $btnOk	    = $tip.find('[data-apply="confirmation"]');
		var $btnCancel  = $tip.find('[data-dismiss="confirmation"]');
		var options	    = this.options

		$btnOk.addClass(this.getBtnOkClass())
			.html(this.getBtnOkLabel())
			.prepend($('<i></i>').addClass(this.getBtnOkIcon()), " ")
			.attr('href', this.getHref())
			.attr('target', this.getTarget())
			.off('click').on('click', function(event) {
				options.onConfirm(event, that.$element);

				that.$element.confirmation('hide');
			});

		$btnCancel.addClass(this.getBtnCancelClass())
			.html(this.getBtnCancelLabel())
			.prepend($('<i></i>').addClass(this.getBtnCancelIcon()), " ")
			.off('click').on('click', function(event){
				options.onCancel(event, that.$element);

				that.$element.confirmation('hide');
			});

		$tip.find('.popover-title')[this.options.html ? 'html' : 'text'](title);

		$tip.removeClass('fade top bottom left right in');

		// IE8 doesn't accept hiding via the `:empty` pseudo selector, we have to do
		// this manually by checking the contents.
		if (!$tip.find('.popover-title').html()) $tip.find('.popover-title').hide();
	}

	Confirmation.prototype.getBtnOkClass = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-btnOkClass') || (typeof o.btnOkClass == 'function' ? o.btnOkClass.call($e[0]) : o.btnOkClass);
	}

	Confirmation.prototype.getBtnOkLabel = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-btnOkLabel') || (typeof o.btnOkLabel == 'function' ? o.btnOkLabel.call($e[0]) : o.btnOkLabel);
	}

	Confirmation.prototype.getBtnOkIcon = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-btnOkIcon') || (typeof o.btnOkIcon == 'function' ?  o.btnOkIcon.call($e[0]) : o.btnOkIcon);
	}

	Confirmation.prototype.getBtnCancelClass = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-btnCancelClass') || (typeof o.btnCancelClass == 'function' ? o.btnCancelClass.call($e[0]) : o.btnCancelClass);
	}

	Confirmation.prototype.getBtnCancelLabel = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-btnCancelLabel') || (typeof o.btnCancelLabel == 'function' ? o.btnCancelLabel.call($e[0]) : o.btnCancelLabel);
	}

	Confirmation.prototype.getBtnCancelIcon = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-btnCancelIcon') || (typeof o.btnCancelIcon == 'function' ? o.btnCancelIcon.call($e[0]) : o.btnCancelIcon);
	}

	Confirmation.prototype.getHref = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-href') || (typeof o.href == 'function' ? o.href.call($e[0]) : o.href);
	}

	Confirmation.prototype.getTarget = function () {
		var $e = this.$element;
		var o  = this.options;

		return $e.attr('data-target') || (typeof o.target == 'function' ? o.target.call($e[0]) : o.target);
	}

	Confirmation.prototype.isPopout = function () {
		var popout;
		var $e = this.$element;
		var o = this.options;

		popout = $e.attr('data-popout') || (typeof o.popout == 'function' ? o.popout.call($e[0]) :	o.popout);

		if(popout == 'false') popout = false;

		return popout
	}


	// CONFIRMATION PLUGIN DEFINITION
	// =========================
	var old = $.fn.confirmation;

	$.fn.confirmation = function (option) {
		var that = this;

		return this.each(function () {
			var $this   = $(this);
			var data    = $this.data('bs.confirmation');
			var options = typeof option == 'object' && option;

			options = options || {};
			options.all_selector = that.selector;

			if (!data && option == 'destroy') return;
			if (!data) $this.data('bs.confirmation', (data = new Confirmation(this, options)));
			if (typeof option == 'string') data[option]();
		});
	}

	$.fn.confirmation.Constructor = Confirmation


	// CONFIRMATION NO CONFLICT
	// ===================
	$.fn.confirmation.noConflict = function () {
		$.fn.confirmation = old;

		return this;
	}
}(jQuery);



!function(){var q=null;window.PR_SHOULD_USE_CONTINUATION=!0;
(function(){function S(a){function d(e){var b=e.charCodeAt(0);if(b!==92)return b;var a=e.charAt(1);return(b=r[a])?b:"0"<=a&&a<="7"?parseInt(e.substring(1),8):a==="u"||a==="x"?parseInt(e.substring(2),16):e.charCodeAt(1)}function g(e){if(e<32)return(e<16?"\\x0":"\\x")+e.toString(16);e=String.fromCharCode(e);return e==="\\"||e==="-"||e==="]"||e==="^"?"\\"+e:e}function b(e){var b=e.substring(1,e.length-1).match(/\\u[\dA-Fa-f]{4}|\\x[\dA-Fa-f]{2}|\\[0-3][0-7]{0,2}|\\[0-7]{1,2}|\\[\S\s]|[^\\]/g),e=[],a=
b[0]==="^",c=["["];a&&c.push("^");for(var a=a?1:0,f=b.length;a<f;++a){var h=b[a];if(/\\[bdsw]/i.test(h))c.push(h);else{var h=d(h),l;a+2<f&&"-"===b[a+1]?(l=d(b[a+2]),a+=2):l=h;e.push([h,l]);l<65||h>122||(l<65||h>90||e.push([Math.max(65,h)|32,Math.min(l,90)|32]),l<97||h>122||e.push([Math.max(97,h)&-33,Math.min(l,122)&-33]))}}e.sort(function(e,a){return e[0]-a[0]||a[1]-e[1]});b=[];f=[];for(a=0;a<e.length;++a)h=e[a],h[0]<=f[1]+1?f[1]=Math.max(f[1],h[1]):b.push(f=h);for(a=0;a<b.length;++a)h=b[a],c.push(g(h[0])),
h[1]>h[0]&&(h[1]+1>h[0]&&c.push("-"),c.push(g(h[1])));c.push("]");return c.join("")}function s(e){for(var a=e.source.match(/\[(?:[^\\\]]|\\[\S\s])*]|\\u[\dA-Fa-f]{4}|\\x[\dA-Fa-f]{2}|\\\d+|\\[^\dux]|\(\?[!:=]|[()^]|[^()[\\^]+/g),c=a.length,d=[],f=0,h=0;f<c;++f){var l=a[f];l==="("?++h:"\\"===l.charAt(0)&&(l=+l.substring(1))&&(l<=h?d[l]=-1:a[f]=g(l))}for(f=1;f<d.length;++f)-1===d[f]&&(d[f]=++x);for(h=f=0;f<c;++f)l=a[f],l==="("?(++h,d[h]||(a[f]="(?:")):"\\"===l.charAt(0)&&(l=+l.substring(1))&&l<=h&&
(a[f]="\\"+d[l]);for(f=0;f<c;++f)"^"===a[f]&&"^"!==a[f+1]&&(a[f]="");if(e.ignoreCase&&m)for(f=0;f<c;++f)l=a[f],e=l.charAt(0),l.length>=2&&e==="["?a[f]=b(l):e!=="\\"&&(a[f]=l.replace(/[A-Za-z]/g,function(a){a=a.charCodeAt(0);return"["+String.fromCharCode(a&-33,a|32)+"]"}));return a.join("")}for(var x=0,m=!1,j=!1,k=0,c=a.length;k<c;++k){var i=a[k];if(i.ignoreCase)j=!0;else if(/[a-z]/i.test(i.source.replace(/\\u[\da-f]{4}|\\x[\da-f]{2}|\\[^UXux]/gi,""))){m=!0;j=!1;break}}for(var r={b:8,t:9,n:10,v:11,
f:12,r:13},n=[],k=0,c=a.length;k<c;++k){i=a[k];if(i.global||i.multiline)throw Error(""+i);n.push("(?:"+s(i)+")")}return RegExp(n.join("|"),j?"gi":"g")}function T(a,d){function g(a){var c=a.nodeType;if(c==1){if(!b.test(a.className)){for(c=a.firstChild;c;c=c.nextSibling)g(c);c=a.nodeName.toLowerCase();if("br"===c||"li"===c)s[j]="\n",m[j<<1]=x++,m[j++<<1|1]=a}}else if(c==3||c==4)c=a.nodeValue,c.length&&(c=d?c.replace(/\r\n?/g,"\n"):c.replace(/[\t\n\r ]+/g," "),s[j]=c,m[j<<1]=x,x+=c.length,m[j++<<1|1]=
a)}var b=/(?:^|\s)nocode(?:\s|$)/,s=[],x=0,m=[],j=0;g(a);return{a:s.join("").replace(/\n$/,""),d:m}}function H(a,d,g,b){d&&(a={a:d,e:a},g(a),b.push.apply(b,a.g))}function U(a){for(var d=void 0,g=a.firstChild;g;g=g.nextSibling)var b=g.nodeType,d=b===1?d?a:g:b===3?V.test(g.nodeValue)?a:d:d;return d===a?void 0:d}function C(a,d){function g(a){for(var j=a.e,k=[j,"pln"],c=0,i=a.a.match(s)||[],r={},n=0,e=i.length;n<e;++n){var z=i[n],w=r[z],t=void 0,f;if(typeof w==="string")f=!1;else{var h=b[z.charAt(0)];
if(h)t=z.match(h[1]),w=h[0];else{for(f=0;f<x;++f)if(h=d[f],t=z.match(h[1])){w=h[0];break}t||(w="pln")}if((f=w.length>=5&&"lang-"===w.substring(0,5))&&!(t&&typeof t[1]==="string"))f=!1,w="src";f||(r[z]=w)}h=c;c+=z.length;if(f){f=t[1];var l=z.indexOf(f),B=l+f.length;t[2]&&(B=z.length-t[2].length,l=B-f.length);w=w.substring(5);H(j+h,z.substring(0,l),g,k);H(j+h+l,f,I(w,f),k);H(j+h+B,z.substring(B),g,k)}else k.push(j+h,w)}a.g=k}var b={},s;(function(){for(var g=a.concat(d),j=[],k={},c=0,i=g.length;c<i;++c){var r=
g[c],n=r[3];if(n)for(var e=n.length;--e>=0;)b[n.charAt(e)]=r;r=r[1];n=""+r;k.hasOwnProperty(n)||(j.push(r),k[n]=q)}j.push(/[\S\s]/);s=S(j)})();var x=d.length;return g}function v(a){var d=[],g=[];a.tripleQuotedStrings?d.push(["str",/^(?:'''(?:[^'\\]|\\[\S\s]|''?(?=[^']))*(?:'''|$)|"""(?:[^"\\]|\\[\S\s]|""?(?=[^"]))*(?:"""|$)|'(?:[^'\\]|\\[\S\s])*(?:'|$)|"(?:[^"\\]|\\[\S\s])*(?:"|$))/,q,"'\""]):a.multiLineStrings?d.push(["str",/^(?:'(?:[^'\\]|\\[\S\s])*(?:'|$)|"(?:[^"\\]|\\[\S\s])*(?:"|$)|`(?:[^\\`]|\\[\S\s])*(?:`|$))/,
q,"'\"`"]):d.push(["str",/^(?:'(?:[^\n\r'\\]|\\.)*(?:'|$)|"(?:[^\n\r"\\]|\\.)*(?:"|$))/,q,"\"'"]);a.verbatimStrings&&g.push(["str",/^@"(?:[^"]|"")*(?:"|$)/,q]);var b=a.hashComments;b&&(a.cStyleComments?(b>1?d.push(["com",/^#(?:##(?:[^#]|#(?!##))*(?:###|$)|.*)/,q,"#"]):d.push(["com",/^#(?:(?:define|e(?:l|nd)if|else|error|ifn?def|include|line|pragma|undef|warning)\b|[^\n\r]*)/,q,"#"]),g.push(["str",/^<(?:(?:(?:\.\.\/)*|\/?)(?:[\w-]+(?:\/[\w-]+)+)?[\w-]+\.h(?:h|pp|\+\+)?|[a-z]\w*)>/,q])):d.push(["com",
/^#[^\n\r]*/,q,"#"]));a.cStyleComments&&(g.push(["com",/^\/\/[^\n\r]*/,q]),g.push(["com",/^\/\*[\S\s]*?(?:\*\/|$)/,q]));if(b=a.regexLiterals){var s=(b=b>1?"":"\n\r")?".":"[\\S\\s]";g.push(["lang-regex",RegExp("^(?:^^\\.?|[+-]|[!=]=?=?|\\#|%=?|&&?=?|\\(|\\*=?|[+\\-]=|->|\\/=?|::?|<<?=?|>>?>?=?|,|;|\\?|@|\\[|~|{|\\^\\^?=?|\\|\\|?=?|break|case|continue|delete|do|else|finally|instanceof|return|throw|try|typeof)\\s*("+("/(?=[^/*"+b+"])(?:[^/\\x5B\\x5C"+b+"]|\\x5C"+s+"|\\x5B(?:[^\\x5C\\x5D"+b+"]|\\x5C"+
s+")*(?:\\x5D|$))+/")+")")])}(b=a.types)&&g.push(["typ",b]);b=(""+a.keywords).replace(/^ | $/g,"");b.length&&g.push(["kwd",RegExp("^(?:"+b.replace(/[\s,]+/g,"|")+")\\b"),q]);d.push(["pln",/^\s+/,q," \r\n\t\u00a0"]);b="^.[^\\s\\w.$@'\"`/\\\\]*";a.regexLiterals&&(b+="(?!s*/)");g.push(["lit",/^@[$_a-z][\w$@]*/i,q],["typ",/^(?:[@_]?[A-Z]+[a-z][\w$@]*|\w+_t\b)/,q],["pln",/^[$_a-z][\w$@]*/i,q],["lit",/^(?:0x[\da-f]+|(?:\d(?:_\d+)*\d*(?:\.\d*)?|\.\d\+)(?:e[+-]?\d+)?)[a-z]*/i,q,"0123456789"],["pln",/^\\[\S\s]?/,
q],["pun",RegExp(b),q]);return C(d,g)}function J(a,d,g){function b(a){var c=a.nodeType;if(c==1&&!x.test(a.className))if("br"===a.nodeName)s(a),a.parentNode&&a.parentNode.removeChild(a);else for(a=a.firstChild;a;a=a.nextSibling)b(a);else if((c==3||c==4)&&g){var d=a.nodeValue,i=d.match(m);if(i)c=d.substring(0,i.index),a.nodeValue=c,(d=d.substring(i.index+i[0].length))&&a.parentNode.insertBefore(j.createTextNode(d),a.nextSibling),s(a),c||a.parentNode.removeChild(a)}}function s(a){function b(a,c){var d=
c?a.cloneNode(!1):a,e=a.parentNode;if(e){var e=b(e,1),g=a.nextSibling;e.appendChild(d);for(var i=g;i;i=g)g=i.nextSibling,e.appendChild(i)}return d}for(;!a.nextSibling;)if(a=a.parentNode,!a)return;for(var a=b(a.nextSibling,0),d;(d=a.parentNode)&&d.nodeType===1;)a=d;c.push(a)}for(var x=/(?:^|\s)nocode(?:\s|$)/,m=/\r\n?|\n/,j=a.ownerDocument,k=j.createElement("li");a.firstChild;)k.appendChild(a.firstChild);for(var c=[k],i=0;i<c.length;++i)b(c[i]);d===(d|0)&&c[0].setAttribute("value",d);var r=j.createElement("ol");
r.className="linenums";for(var d=Math.max(0,d-1|0)||0,i=0,n=c.length;i<n;++i)k=c[i],k.className="L"+(i+d)%10,k.firstChild||k.appendChild(j.createTextNode("\u00a0")),r.appendChild(k);a.appendChild(r)}function p(a,d){for(var g=d.length;--g>=0;){var b=d[g];F.hasOwnProperty(b)?D.console&&console.warn("cannot override language handler %s",b):F[b]=a}}function I(a,d){if(!a||!F.hasOwnProperty(a))a=/^\s*</.test(d)?"default-markup":"default-code";return F[a]}function K(a){var d=a.h;try{var g=T(a.c,a.i),b=g.a;
a.a=b;a.d=g.d;a.e=0;I(d,b)(a);var s=/\bMSIE\s(\d+)/.exec(navigator.userAgent),s=s&&+s[1]<=8,d=/\n/g,x=a.a,m=x.length,g=0,j=a.d,k=j.length,b=0,c=a.g,i=c.length,r=0;c[i]=m;var n,e;for(e=n=0;e<i;)c[e]!==c[e+2]?(c[n++]=c[e++],c[n++]=c[e++]):e+=2;i=n;for(e=n=0;e<i;){for(var p=c[e],w=c[e+1],t=e+2;t+2<=i&&c[t+1]===w;)t+=2;c[n++]=p;c[n++]=w;e=t}c.length=n;var f=a.c,h;if(f)h=f.style.display,f.style.display="none";try{for(;b<k;){var l=j[b+2]||m,B=c[r+2]||m,t=Math.min(l,B),A=j[b+1],G;if(A.nodeType!==1&&(G=x.substring(g,
t))){s&&(G=G.replace(d,"\r"));A.nodeValue=G;var L=A.ownerDocument,o=L.createElement("span");o.className=c[r+1];var v=A.parentNode;v.replaceChild(o,A);o.appendChild(A);g<l&&(j[b+1]=A=L.createTextNode(x.substring(t,l)),v.insertBefore(A,o.nextSibling))}g=t;g>=l&&(b+=2);g>=B&&(r+=2)}}finally{if(f)f.style.display=h}}catch(u){D.console&&console.log(u&&u.stack||u)}}var D=window,y=["break,continue,do,else,for,if,return,while"],E=[[y,"auto,case,char,const,default,double,enum,extern,float,goto,inline,int,long,register,short,signed,sizeof,static,struct,switch,typedef,union,unsigned,void,volatile"],
"catch,class,delete,false,import,new,operator,private,protected,public,this,throw,true,try,typeof"],M=[E,"alignof,align_union,asm,axiom,bool,concept,concept_map,const_cast,constexpr,decltype,delegate,dynamic_cast,explicit,export,friend,generic,late_check,mutable,namespace,nullptr,property,reinterpret_cast,static_assert,static_cast,template,typeid,typename,using,virtual,where"],N=[E,"abstract,assert,boolean,byte,extends,final,finally,implements,import,instanceof,interface,null,native,package,strictfp,super,synchronized,throws,transient"],
O=[N,"as,base,by,checked,decimal,delegate,descending,dynamic,event,fixed,foreach,from,group,implicit,in,internal,into,is,let,lock,object,out,override,orderby,params,partial,readonly,ref,sbyte,sealed,stackalloc,string,select,uint,ulong,unchecked,unsafe,ushort,var,virtual,where"],E=[E,"debugger,eval,export,function,get,null,set,undefined,var,with,Infinity,NaN"],P=[y,"and,as,assert,class,def,del,elif,except,exec,finally,from,global,import,in,is,lambda,nonlocal,not,or,pass,print,raise,try,with,yield,False,True,None"],
Q=[y,"alias,and,begin,case,class,def,defined,elsif,end,ensure,false,in,module,next,nil,not,or,redo,rescue,retry,self,super,then,true,undef,unless,until,when,yield,BEGIN,END"],W=[y,"as,assert,const,copy,drop,enum,extern,fail,false,fn,impl,let,log,loop,match,mod,move,mut,priv,pub,pure,ref,self,static,struct,true,trait,type,unsafe,use"],y=[y,"case,done,elif,esac,eval,fi,function,in,local,set,then,until"],R=/^(DIR|FILE|vector|(de|priority_)?queue|list|stack|(const_)?iterator|(multi)?(set|map)|bitset|u?(int|float)\d*)\b/,
V=/\S/,X=v({keywords:[M,O,E,"caller,delete,die,do,dump,elsif,eval,exit,foreach,for,goto,if,import,last,local,my,next,no,our,print,package,redo,require,sub,undef,unless,until,use,wantarray,while,BEGIN,END",P,Q,y],hashComments:!0,cStyleComments:!0,multiLineStrings:!0,regexLiterals:!0}),F={};p(X,["default-code"]);p(C([],[["pln",/^[^<?]+/],["dec",/^<!\w[^>]*(?:>|$)/],["com",/^<\!--[\S\s]*?(?:--\>|$)/],["lang-",/^<\?([\S\s]+?)(?:\?>|$)/],["lang-",/^<%([\S\s]+?)(?:%>|$)/],["pun",/^(?:<[%?]|[%?]>)/],["lang-",
/^<xmp\b[^>]*>([\S\s]+?)<\/xmp\b[^>]*>/i],["lang-js",/^<script\b[^>]*>([\S\s]*?)(<\/script\b[^>]*>)/i],["lang-css",/^<style\b[^>]*>([\S\s]*?)(<\/style\b[^>]*>)/i],["lang-in.tag",/^(<\/?[a-z][^<>]*>)/i]]),["default-markup","htm","html","mxml","xhtml","xml","xsl"]);p(C([["pln",/^\s+/,q," \t\r\n"],["atv",/^(?:"[^"]*"?|'[^']*'?)/,q,"\"'"]],[["tag",/^^<\/?[a-z](?:[\w-.:]*\w)?|\/?>$/i],["atn",/^(?!style[\s=]|on)[a-z](?:[\w:-]*\w)?/i],["lang-uq.val",/^=\s*([^\s"'>]*(?:[^\s"'/>]|\/(?=\s)))/],["pun",/^[/<->]+/],
["lang-js",/^on\w+\s*=\s*"([^"]+)"/i],["lang-js",/^on\w+\s*=\s*'([^']+)'/i],["lang-js",/^on\w+\s*=\s*([^\s"'>]+)/i],["lang-css",/^style\s*=\s*"([^"]+)"/i],["lang-css",/^style\s*=\s*'([^']+)'/i],["lang-css",/^style\s*=\s*([^\s"'>]+)/i]]),["in.tag"]);p(C([],[["atv",/^[\S\s]+/]]),["uq.val"]);p(v({keywords:M,hashComments:!0,cStyleComments:!0,types:R}),["c","cc","cpp","cxx","cyc","m"]);p(v({keywords:"null,true,false"}),["json"]);p(v({keywords:O,hashComments:!0,cStyleComments:!0,verbatimStrings:!0,types:R}),
["cs"]);p(v({keywords:N,cStyleComments:!0}),["java"]);p(v({keywords:y,hashComments:!0,multiLineStrings:!0}),["bash","bsh","csh","sh"]);p(v({keywords:P,hashComments:!0,multiLineStrings:!0,tripleQuotedStrings:!0}),["cv","py","python"]);p(v({keywords:"caller,delete,die,do,dump,elsif,eval,exit,foreach,for,goto,if,import,last,local,my,next,no,our,print,package,redo,require,sub,undef,unless,until,use,wantarray,while,BEGIN,END",hashComments:!0,multiLineStrings:!0,regexLiterals:2}),["perl","pl","pm"]);p(v({keywords:Q,
hashComments:!0,multiLineStrings:!0,regexLiterals:!0}),["rb","ruby"]);p(v({keywords:E,cStyleComments:!0,regexLiterals:!0}),["javascript","js"]);p(v({keywords:"all,and,by,catch,class,else,extends,false,finally,for,if,in,is,isnt,loop,new,no,not,null,of,off,on,or,return,super,then,throw,true,try,unless,until,when,while,yes",hashComments:3,cStyleComments:!0,multilineStrings:!0,tripleQuotedStrings:!0,regexLiterals:!0}),["coffee"]);p(v({keywords:W,cStyleComments:!0,multilineStrings:!0}),["rc","rs","rust"]);
p(C([],[["str",/^[\S\s]+/]]),["regex"]);var Y=D.PR={createSimpleLexer:C,registerLangHandler:p,sourceDecorator:v,PR_ATTRIB_NAME:"atn",PR_ATTRIB_VALUE:"atv",PR_COMMENT:"com",PR_DECLARATION:"dec",PR_KEYWORD:"kwd",PR_LITERAL:"lit",PR_NOCODE:"nocode",PR_PLAIN:"pln",PR_PUNCTUATION:"pun",PR_SOURCE:"src",PR_STRING:"str",PR_TAG:"tag",PR_TYPE:"typ",prettyPrintOne:D.prettyPrintOne=function(a,d,g){var b=document.createElement("div");b.innerHTML="<pre>"+a+"</pre>";b=b.firstChild;g&&J(b,g,!0);K({h:d,j:g,c:b,i:1});
return b.innerHTML},prettyPrint:D.prettyPrint=function(a,d){function g(){for(var b=D.PR_SHOULD_USE_CONTINUATION?c.now()+250:Infinity;i<p.length&&c.now()<b;i++){for(var d=p[i],j=h,k=d;k=k.previousSibling;){var m=k.nodeType,o=(m===7||m===8)&&k.nodeValue;if(o?!/^\??prettify\b/.test(o):m!==3||/\S/.test(k.nodeValue))break;if(o){j={};o.replace(/\b(\w+)=([\w%+\-.:]+)/g,function(a,b,c){j[b]=c});break}}k=d.className;if((j!==h||e.test(k))&&!v.test(k)){m=!1;for(o=d.parentNode;o;o=o.parentNode)if(f.test(o.tagName)&&
o.className&&e.test(o.className)){m=!0;break}if(!m){d.className+=" prettyprinted";m=j.lang;if(!m){var m=k.match(n),y;if(!m&&(y=U(d))&&t.test(y.tagName))m=y.className.match(n);m&&(m=m[1])}if(w.test(d.tagName))o=1;else var o=d.currentStyle,u=s.defaultView,o=(o=o?o.whiteSpace:u&&u.getComputedStyle?u.getComputedStyle(d,q).getPropertyValue("white-space"):0)&&"pre"===o.substring(0,3);u=j.linenums;if(!(u=u==="true"||+u))u=(u=k.match(/\blinenums\b(?::(\d+))?/))?u[1]&&u[1].length?+u[1]:!0:!1;u&&J(d,u,o);r=
{h:m,c:d,j:u,i:o};K(r)}}}i<p.length?setTimeout(g,250):"function"===typeof a&&a()}for(var b=d||document.body,s=b.ownerDocument||document,b=[b.getElementsByTagName("pre"),b.getElementsByTagName("code"),b.getElementsByTagName("xmp")],p=[],m=0;m<b.length;++m)for(var j=0,k=b[m].length;j<k;++j)p.push(b[m][j]);var b=q,c=Date;c.now||(c={now:function(){return+new Date}});var i=0,r,n=/\blang(?:uage)?-([\w.]+)(?!\S)/,e=/\bprettyprint\b/,v=/\bprettyprinted\b/,w=/pre|xmp/i,t=/^code$/i,f=/^(?:pre|code|xmp)$/i,
h={};g()}};typeof define==="function"&&define.amd&&define("google-code-prettify",[],function(){return Y})})();}()

// lang-apollo.js
PR.registerLangHandler(PR.createSimpleLexer([["com",/^#[^\n\r]*/,null,"#"],["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^"(?:[^"\\]|\\[\S\s])*(?:"|$)/,null,'"']],[["kwd",/^(?:ADS|AD|AUG|BZF|BZMF|CAE|CAF|CA|CCS|COM|CS|DAS|DCA|DCOM|DCS|DDOUBL|DIM|DOUBLE|DTCB|DTCF|DV|DXCH|EDRUPT|EXTEND|INCR|INDEX|NDX|INHINT|LXCH|MASK|MSK|MP|MSU|NOOP|OVSK|QXCH|RAND|READ|RELINT|RESUME|RETURN|ROR|RXOR|SQUARE|SU|TCR|TCAA|OVSK|TCF|TC|TS|WAND|WOR|WRITE|XCH|XLQ|XXALQ|ZL|ZQ|ADD|ADZ|SUB|SUZ|MPY|MPR|MPZ|DVP|COM|ABS|CLA|CLZ|LDQ|STO|STQ|ALS|LLS|LRS|TRA|TSQ|TMI|TOV|AXT|TIX|DLY|INP|OUT)\s/,
null],["typ",/^(?:-?GENADR|=MINUS|2BCADR|VN|BOF|MM|-?2CADR|-?[1-6]DNADR|ADRES|BBCON|[ES]?BANK=?|BLOCK|BNKSUM|E?CADR|COUNT\*?|2?DEC\*?|-?DNCHAN|-?DNPTR|EQUALS|ERASE|MEMORY|2?OCT|REMADR|SETLOC|SUBRO|ORG|BSS|BES|SYN|EQU|DEFINE|END)\s/,null],["lit",/^'(?:-*(?:\w|\\[!-~])(?:[\w-]*|\\[!-~])[!=?]?)?/],["pln",/^-*(?:[!-z]|\\[!-~])(?:[\w-]*|\\[!-~])[!=?]?/],["pun",/^[^\w\t\n\r "'-);\\\xa0]+/]]),["apollo","agc","aea"]);

// lang-basic.js
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["str",/^"(?:[^\n\r"\\]|\\.)*(?:"|$)/,a,'"'],["pln",/^\s+/,a," \r\n\t\u00a0"]],[["com",/^REM[^\n\r]*/,a],["kwd",/^\b(?:AND|CLOSE|CLR|CMD|CONT|DATA|DEF ?FN|DIM|END|FOR|GET|GOSUB|GOTO|IF|INPUT|LET|LIST|LOAD|NEW|NEXT|NOT|ON|OPEN|OR|POKE|PRINT|READ|RESTORE|RETURN|RUN|SAVE|STEP|STOP|SYS|THEN|TO|VERIFY|WAIT)\b/,a],["pln",/^[a-z][^\W_]?(?:\$|%)?/i,a],["lit",/^(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?/i,a,"0123456789"],["pun",
/^.[^\s\w"$%.]*/,a]]),["basic","cbm"]);

// lang-clj.js
/*
 Copyright (C) 2011 Google Inc.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["opn",/^[([{]+/,a,"([{"],["clo",/^[)\]}]+/,a,")]}"],["com",/^;[^\n\r]*/,a,";"],["pln",/^[\t\n\r \xa0]+/,a,"\t\n\r \u00a0"],["str",/^"(?:[^"\\]|\\[\S\s])*(?:"|$)/,a,'"']],[["kwd",/^(?:def|if|do|let|quote|var|fn|loop|recur|throw|try|monitor-enter|monitor-exit|defmacro|defn|defn-|macroexpand|macroexpand-1|for|doseq|dosync|dotimes|and|or|when|not|assert|doto|proxy|defstruct|first|rest|cons|defprotocol|deftype|defrecord|reify|defmulti|defmethod|meta|with-meta|ns|in-ns|create-ns|import|intern|refer|alias|namespace|resolve|ref|deref|refset|new|set!|memfn|to-array|into-array|aset|gen-class|reduce|map|filter|find|nil?|empty?|hash-map|hash-set|vec|vector|seq|flatten|reverse|assoc|dissoc|list|list?|disj|get|union|difference|intersection|extend|extend-type|extend-protocol|prn)\b/,a],
["typ",/^:[\dA-Za-z-]+/]]),["clj"]);

// lang-css.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\f\r ]+/,null," \t\r\n\u000c"]],[["str",/^"(?:[^\n\f\r"\\]|\\(?:\r\n?|\n|\f)|\\[\S\s])*"/,null],["str",/^'(?:[^\n\f\r'\\]|\\(?:\r\n?|\n|\f)|\\[\S\s])*'/,null],["lang-css-str",/^url\(([^"')]+)\)/i],["kwd",/^(?:url|rgb|!important|@import|@page|@media|@charset|inherit)(?=[^\w-]|$)/i,null],["lang-css-kw",/^(-?(?:[_a-z]|\\[\da-f]+ ?)(?:[\w-]|\\\\[\da-f]+ ?)*)\s*:/i],["com",/^\/\*[^*]*\*+(?:[^*/][^*]*\*+)*\//],
["com",/^(?:<\!--|--\>)/],["lit",/^(?:\d+|\d*\.\d+)(?:%|[a-z]+)?/i],["lit",/^#[\da-f]{3,6}\b/i],["pln",/^-?(?:[_a-z]|\\[\da-f]+ ?)(?:[\w-]|\\\\[\da-f]+ ?)*/i],["pun",/^[^\s\w"']+/]]),["css"]);PR.registerLangHandler(PR.createSimpleLexer([],[["kwd",/^-?(?:[_a-z]|\\[\da-f]+ ?)(?:[\w-]|\\\\[\da-f]+ ?)*/i]]),["css-kw"]);PR.registerLangHandler(PR.createSimpleLexer([],[["str",/^[^"')]+/]]),["css-str"]);

// lang-dart.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"]],[["com",/^#!.*/],["kwd",/^\b(?:import|library|part of|part|as|show|hide)\b/i],["com",/^\/\/.*/],["com",/^\/\*[^*]*\*+(?:[^*/][^*]*\*+)*\//],["kwd",/^\b(?:class|interface)\b/i],["kwd",/^\b(?:assert|break|case|catch|continue|default|do|else|finally|for|if|in|is|new|return|super|switch|this|throw|try|while)\b/i],["kwd",/^\b(?:abstract|const|extends|factory|final|get|implements|native|operator|set|static|typedef|var)\b/i],
["typ",/^\b(?:bool|double|dynamic|int|num|object|string|void)\b/i],["kwd",/^\b(?:false|null|true)\b/i],["str",/^r?'''[\S\s]*?[^\\]'''/],["str",/^r?"""[\S\s]*?[^\\]"""/],["str",/^r?'('|[^\n\f\r]*?[^\\]')/],["str",/^r?"("|[^\n\f\r]*?[^\\]")/],["pln",/^[$_a-z]\w*/i],["pun",/^[!%&*+/:<-?^|~-]/],["lit",/^\b0x[\da-f]+/i],["lit",/^\b\d+(?:\.\d*)?(?:e[+-]?\d+)?/i],["lit",/^\b\.\d+(?:e[+-]?\d+)?/i],["pun",/^[(),.;[\]{}]/]]),
["dart"]);

// lang-erlang.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t-\r ]+/,null,"\t\n\u000b\u000c\r "],["str",/^"(?:[^\n\f\r"\\]|\\[\S\s])*(?:"|$)/,null,'"'],["lit",/^[a-z]\w*/],["lit",/^'(?:[^\n\f\r'\\]|\\[^&])+'?/,null,"'"],["lit",/^\?[^\t\n ({]+/,null,"?"],["lit",/^(?:0o[0-7]+|0x[\da-f]+|\d+(?:\.\d+)?(?:e[+-]?\d+)?)/i,null,"0123456789"]],[["com",/^%[^\n]*/],["kwd",/^(?:module|attributes|do|let|in|letrec|apply|call|primop|case|of|end|when|fun|try|catch|receive|after|char|integer|float,atom,string,var)\b/],
["kwd",/^-[_a-z]+/],["typ",/^[A-Z_]\w*/],["pun",/^[,.;]/]]),["erlang","erl"]);

// lang-go.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["pln",/^(?:"(?:[^"\\]|\\[\S\s])*(?:"|$)|'(?:[^'\\]|\\[\S\s])+(?:'|$)|`[^`]*(?:`|$))/,null,"\"'"]],[["com",/^(?:\/\/[^\n\r]*|\/\*[\S\s]*?\*\/)/],["pln",/^(?:[^"'/`]|\/(?![*/]))+/]]),["go"]);

// lang-hs.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t-\r ]+/,null,"\t\n\u000b\u000c\r "],["str",/^"(?:[^\n\f\r"\\]|\\[\S\s])*(?:"|$)/,null,'"'],["str",/^'(?:[^\n\f\r'\\]|\\[^&])'?/,null,"'"],["lit",/^(?:0o[0-7]+|0x[\da-f]+|\d+(?:\.\d+)?(?:e[+-]?\d+)?)/i,null,"0123456789"]],[["com",/^(?:--+[^\n\f\r]*|{-(?:[^-]|-+[^}-])*-})/],["kwd",/^(?:case|class|data|default|deriving|do|else|if|import|in|infix|infixl|infixr|instance|let|module|newtype|of|then|type|where|_)(?=[^\d'A-Za-z]|$)/,
null],["pln",/^(?:[A-Z][\w']*\.)*[A-Za-z][\w']*/],["pun",/^[^\d\t-\r "'A-Za-z]+/]]),["hs"]);

// lang-lisp.js
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["opn",/^\(+/,a,"("],["clo",/^\)+/,a,")"],["com",/^;[^\n\r]*/,a,";"],["pln",/^[\t\n\r \xa0]+/,a,"\t\n\r \u00a0"],["str",/^"(?:[^"\\]|\\[\S\s])*(?:"|$)/,a,'"']],[["kwd",/^(?:block|c[ad]+r|catch|con[ds]|def(?:ine|un)|do|eq|eql|equal|equalp|eval-when|flet|format|go|if|labels|lambda|let|load-time-value|locally|macrolet|multiple-value-call|nil|progn|progv|quote|require|return-from|setq|symbol-macrolet|t|tagbody|the|throw|unwind)\b/,a],
["lit",/^[+-]?(?:[#0]x[\da-f]+|\d+\/\d+|(?:\.\d+|\d+(?:\.\d*)?)(?:[de][+-]?\d+)?)/i],["lit",/^'(?:-*(?:\w|\\[!-~])(?:[\w-]*|\\[!-~])[!=?]?)?/],["pln",/^-*(?:[_a-z]|\\[!-~])(?:[\w-]*|\\[!-~])[!=?]?/i],["pun",/^[^\w\t\n\r "'-);\\\xa0]+/]]),["cl","el","lisp","lsp","scm","ss","rkt"]);

// lang-llvm.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^!?"(?:[^"\\]|\\[\S\s])*(?:"|$)/,null,'"'],["com",/^;[^\n\r]*/,null,";"]],[["pln",/^[!%@](?:[$\-.A-Z_a-z][\w$\-.]*|\d+)/],["kwd",/^[^\W\d]\w*/,null],["lit",/^\d+\.\d+/],["lit",/^(?:\d+|0[Xx][\dA-Fa-f]+)/],["pun",/^[(-*,:<->[\]{}]|\.\.\.$/]]),["llvm","ll"]);

// lang-lua.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^(?:"(?:[^"\\]|\\[\S\s])*(?:"|$)|'(?:[^'\\]|\\[\S\s])*(?:'|$))/,null,"\"'"]],[["com",/^--(?:\[(=*)\[[\S\s]*?(?:]\1]|$)|[^\n\r]*)/],["str",/^\[(=*)\[[\S\s]*?(?:]\1]|$)/],["kwd",/^(?:and|break|do|else|elseif|end|false|for|function|if|in|local|nil|not|or|repeat|return|then|true|until|while)\b/,null],["lit",/^[+-]?(?:0x[\da-f]+|(?:\.\d+|\d+(?:\.\d*)?)(?:e[+-]?\d+)?)/i],
["pln",/^[_a-z]\w*/i],["pun",/^[^\w\t\n\r \xa0][^\w\t\n\r "'+=\xa0-]*/]]),["lua"]);

// lang-ml.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["com",/^#(?:if[\t\n\r \xa0]+(?:[$_a-z][\w']*|``[^\t\n\r`]*(?:``|$))|else|endif|light)/i,null,"#"],["str",/^(?:"(?:[^"\\]|\\[\S\s])*(?:"|$)|'(?:[^'\\]|\\[\S\s])(?:'|$))/,null,"\"'"]],[["com",/^(?:\/\/[^\n\r]*|\(\*[\S\s]*?\*\))/],["kwd",/^(?:abstract|and|as|assert|begin|class|default|delegate|do|done|downcast|downto|elif|else|end|exception|extern|false|finally|for|fun|function|if|in|inherit|inline|interface|internal|lazy|let|match|member|module|mutable|namespace|new|null|of|open|or|override|private|public|rec|return|static|struct|then|to|true|try|type|upcast|use|val|void|when|while|with|yield|asr|land|lor|lsl|lsr|lxor|mod|sig|atomic|break|checked|component|const|constraint|constructor|continue|eager|event|external|fixed|functor|global|include|method|mixin|object|parallel|process|protected|pure|sealed|trait|virtual|volatile)\b/],
["lit",/^[+-]?(?:0x[\da-f]+|(?:\.\d+|\d+(?:\.\d*)?)(?:e[+-]?\d+)?)/i],["pln",/^(?:[_a-z][\w']*[!#?]?|``[^\t\n\r`]*(?:``|$))/i],["pun",/^[^\w\t\n\r "'\xa0]+/]]),["fs","ml"]);

// lang-mumps.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^"(?:[^"]|\\.)*"/,null,'"']],[["com",/^;[^\n\r]*/,null,";"],["dec",/^\$(?:d|device|ec|ecode|es|estack|et|etrap|h|horolog|i|io|j|job|k|key|p|principal|q|quit|st|stack|s|storage|sy|system|t|test|tl|tlevel|tr|trestart|x|y|z[a-z]*|a|ascii|c|char|d|data|e|extract|f|find|fn|fnumber|g|get|j|justify|l|length|na|name|o|order|p|piece|ql|qlength|qs|qsubscript|q|query|r|random|re|reverse|s|select|st|stack|t|text|tr|translate|nan)\b/i,
null],["kwd",/^(?:[^$]b|break|c|close|d|do|e|else|f|for|g|goto|h|halt|h|hang|i|if|j|job|k|kill|l|lock|m|merge|n|new|o|open|q|quit|r|read|s|set|tc|tcommit|tre|trestart|tro|trollback|ts|tstart|u|use|v|view|w|write|x|xecute)\b/i,null],["lit",/^[+-]?(?:\.\d+|\d+(?:\.\d*)?)(?:e[+-]?\d+)?/i],["pln",/^[a-z][^\W_]*/i],["pun",/^[^\w\t\n\r"$%;^\xa0]|_/]]),["mumps"]);

// lang-n.js
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["str",/^(?:'(?:[^\n\r'\\]|\\.)*'|"(?:[^\n\r"\\]|\\.)*(?:"|$))/,a,'"'],["com",/^#(?:(?:define|elif|else|endif|error|ifdef|include|ifndef|line|pragma|undef|warning)\b|[^\n\r]*)/,a,"#"],["pln",/^\s+/,a," \r\n\t\u00a0"]],[["str",/^@"(?:[^"]|"")*(?:"|$)/,a],["str",/^<#[^#>]*(?:#>|$)/,a],["str",/^<(?:(?:(?:\.\.\/)*|\/?)(?:[\w-]+(?:\/[\w-]+)+)?[\w-]+\.h|[a-z]\w*)>/,a],["com",/^\/\/[^\n\r]*/,a],["com",/^\/\*[\S\s]*?(?:\*\/|$)/,
a],["kwd",/^(?:abstract|and|as|base|catch|class|def|delegate|enum|event|extern|false|finally|fun|implements|interface|internal|is|macro|match|matches|module|mutable|namespace|new|null|out|override|params|partial|private|protected|public|ref|sealed|static|struct|syntax|this|throw|true|try|type|typeof|using|variant|virtual|volatile|when|where|with|assert|assert2|async|break|checked|continue|do|else|ensures|for|foreach|if|late|lock|new|nolate|otherwise|regexp|repeat|requires|return|surroundwith|unchecked|unless|using|while|yield)\b/,
a],["typ",/^(?:array|bool|byte|char|decimal|double|float|int|list|long|object|sbyte|short|string|ulong|uint|ufloat|ulong|ushort|void)\b/,a],["lit",/^@[$_a-z][\w$@]*/i,a],["typ",/^@[A-Z]+[a-z][\w$@]*/,a],["pln",/^'?[$_a-z][\w$@]*/i,a],["lit",/^(?:0x[\da-f]+|(?:\d(?:_\d+)*\d*(?:\.\d*)?|\.\d\+)(?:e[+-]?\d+)?)[a-z]*/i,a,"0123456789"],["pun",/^.[^\s\w"-$'./@`]*/,a]]),["n","nemerle"]);

// lang-pascal.js
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["str",/^'(?:[^\n\r'\\]|\\.)*(?:'|$)/,a,"'"],["pln",/^\s+/,a," \r\n\t\u00a0"]],[["com",/^\(\*[\S\s]*?(?:\*\)|$)|^{[\S\s]*?(?:}|$)/,a],["kwd",/^(?:absolute|and|array|asm|assembler|begin|case|const|constructor|destructor|div|do|downto|else|end|external|for|forward|function|goto|if|implementation|in|inline|interface|interrupt|label|mod|not|object|of|or|packed|procedure|program|record|repeat|set|shl|shr|then|to|type|unit|until|uses|var|virtual|while|with|xor)\b/i,a],
["lit",/^(?:true|false|self|nil)/i,a],["pln",/^[a-z][^\W_]*/i,a],["lit",/^(?:\$[\da-f]+|(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?)/i,a,"0123456789"],["pun",/^.[^\s\w$'./@]*/,a]]),["pascal"]);

// lang-proto.js
PR.registerLangHandler(PR.sourceDecorator({keywords:"bytes,default,double,enum,extend,extensions,false,group,import,max,message,option,optional,package,repeated,required,returns,rpc,service,syntax,to,true",types:/^(bool|(double|s?fixed|[su]?int)(32|64)|float|string)\b/,cStyleComments:!0}),["proto"]);

// lang-r.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^"(?:[^"\\]|\\[\S\s])*(?:"|$)/,null,'"'],["str",/^'(?:[^'\\]|\\[\S\s])*(?:'|$)/,null,"'"]],[["com",/^#.*/],["kwd",/^(?:if|else|for|while|repeat|in|next|break|return|switch|function)(?![\w.])/],["lit",/^0[Xx][\dA-Fa-f]+([Pp]\d+)?[Li]?/],["lit",/^[+-]?(\d+(\.\d+)?|\.\d+)([Ee][+-]?\d+)?[Li]?/],["lit",/^(?:NULL|NA(?:_(?:integer|real|complex|character)_)?|Inf|TRUE|FALSE|NaN|\.\.(?:\.|\d+))(?![\w.])/],
["pun",/^(?:<<?-|->>?|-|==|<=|>=|<|>|&&?|!=|\|\|?|[!*+/^]|%.*?%|[$=@~]|:{1,3}|[(),;?[\]{}])/],["pln",/^(?:[A-Za-z]+[\w.]*|\.[^\W\d][\w.]*)(?![\w.])/],["str",/^`.+`/]]),["r","s","R","S","Splus"]);

// lang-rd.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["com",/^%[^\n\r]*/,null,"%"]],[["lit",/^\\(?:cr|l?dots|R|tab)\b/],["kwd",/^\\[@-Za-z]+/],["kwd",/^#(?:ifn?def|endif)/],["pln",/^\\[{}]/],["pun",/^[()[\]{}]+/]]),["Rd","rd"]);

// lang-scala.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^"(?:""(?:""?(?!")|[^"\\]|\\.)*"{0,3}|(?:[^\n\r"\\]|\\.)*"?)/,null,'"'],["lit",/^`(?:[^\n\r\\`]|\\.)*`?/,null,"`"],["pun",/^[!#%&(--:-@[-^{-~]+/,null,"!#%&()*+,-:;<=>?@[\\]^{|}~"]],[["str",/^'(?:[^\n\r'\\]|\\(?:'|[^\n\r']+))'/],["lit",/^'[$A-Z_a-z][\w$]*(?![\w$'])/],["kwd",/^(?:abstract|case|catch|class|def|do|else|extends|final|finally|for|forSome|if|implicit|import|lazy|match|new|object|override|package|private|protected|requires|return|sealed|super|throw|trait|try|type|val|var|while|with|yield)\b/],
["lit",/^(?:true|false|null|this)\b/],["lit",/^(?:0(?:[0-7]+|x[\da-f]+)l?|(?:0|[1-9]\d*)(?:(?:\.\d+)?(?:e[+-]?\d+)?f?|l?)|\\.\d+(?:e[+-]?\d+)?f?)/i],["typ",/^[$_]*[A-Z][\d$A-Z_]*[a-z][\w$]*/],["pln",/^[$A-Z_a-z][\w$]*/],["com",/^\/(?:\/.*|\*(?:\/|\**[^*/])*(?:\*+\/?)?)/],["pun",/^(?:\.+|\/)/]]),["scala"]);

// lang-sql.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["str",/^(?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')/,null,"\"'"]],[["com",/^(?:--[^\n\r]*|\/\*[\S\s]*?(?:\*\/|$))/],["kwd",/^(?:add|all|alter|and|any|apply|as|asc|authorization|backup|begin|between|break|browse|bulk|by|cascade|case|check|checkpoint|close|clustered|coalesce|collate|column|commit|compute|connect|constraint|contains|containstable|continue|convert|create|cross|current|current_date|current_time|current_timestamp|current_user|cursor|database|dbcc|deallocate|declare|default|delete|deny|desc|disk|distinct|distributed|double|drop|dummy|dump|else|end|errlvl|escape|except|exec|execute|exists|exit|fetch|file|fillfactor|following|for|foreign|freetext|freetexttable|from|full|function|goto|grant|group|having|holdlock|identity|identitycol|identity_insert|if|in|index|inner|insert|intersect|into|is|join|key|kill|left|like|lineno|load|match|matched|merge|natural|national|nocheck|nonclustered|nocycle|not|null|nullif|of|off|offsets|on|open|opendatasource|openquery|openrowset|openxml|option|or|order|outer|over|partition|percent|pivot|plan|preceding|precision|primary|print|proc|procedure|public|raiserror|read|readtext|reconfigure|references|replication|restore|restrict|return|revoke|right|rollback|rowcount|rowguidcol|rows?|rule|save|schema|select|session_user|set|setuser|shutdown|some|start|statistics|system_user|table|textsize|then|to|top|tran|transaction|trigger|truncate|tsequal|unbounded|union|unique|unpivot|update|updatetext|use|user|using|values|varying|view|waitfor|when|where|while|with|within|writetext|xml)(?=[^\w-]|$)/i,
null],["lit",/^[+-]?(?:0x[\da-f]+|(?:\.\d+|\d+(?:\.\d*)?)(?:e[+-]?\d+)?)/i],["pln",/^[_a-z][\w-]*/i],["pun",/^[^\w\t\n\r "'\xa0][^\w\t\n\r "'+\xa0-]*/]]),["sql"]);

// lang-tcl.js
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["opn",/^{+/,a,"{"],["clo",/^}+/,a,"}"],["com",/^#[^\n\r]*/,a,"#"],["pln",/^[\t\n\r \xa0]+/,a,"\t\n\r \u00a0"],["str",/^"(?:[^"\\]|\\[\S\s])*(?:"|$)/,a,'"']],[["kwd",/^(?:after|append|apply|array|break|case|catch|continue|error|eval|exec|exit|expr|for|foreach|if|incr|info|proc|return|set|switch|trace|uplevel|upvar|while)\b/,a],["lit",/^[+-]?(?:[#0]x[\da-f]+|\d+\/\d+|(?:\.\d+|\d+(?:\.\d*)?)(?:[de][+-]?\d+)?)/i],["lit",
/^'(?:-*(?:\w|\\[!-~])(?:[\w-]*|\\[!-~])[!=?]?)?/],["pln",/^-*(?:[_a-z]|\\[!-~])(?:[\w-]*|\\[!-~])[!=?]?/i],["pun",/^[^\w\t\n\r "'-);\\\xa0]+/]]),["tcl"]);

// lang-tex.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"],["com",/^%[^\n\r]*/,null,"%"]],[["kwd",/^\\[@-Za-z]+/],["kwd",/^\\./],["typ",/^[$&]/],["lit",/[+-]?(?:\.\d+|\d+(?:\.\d*)?)(cm|em|ex|in|pc|pt|bp|mm)/i],["pun",/^[()=[\]{}]+/]]),["latex","tex"]);

// lang-vb.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0\u2028\u2029]+/,null,"\t\n\r \u00a0\u2028\u2029"],["str",/^(?:["\u201c\u201d](?:[^"\u201c\u201d]|["\u201c\u201d]{2})(?:["\u201c\u201d]c|$)|["\u201c\u201d](?:[^"\u201c\u201d]|["\u201c\u201d]{2})*(?:["\u201c\u201d]|$))/i,null,'"\u201c\u201d'],["com",/^['\u2018\u2019](?:_(?:\r\n?|[^\r]?)|[^\n\r_\u2028\u2029])*/,null,"'\u2018\u2019"]],[["kwd",/^(?:addhandler|addressof|alias|and|andalso|ansi|as|assembly|auto|boolean|byref|byte|byval|call|case|catch|cbool|cbyte|cchar|cdate|cdbl|cdec|char|cint|class|clng|cobj|const|cshort|csng|cstr|ctype|date|decimal|declare|default|delegate|dim|directcast|do|double|each|else|elseif|end|endif|enum|erase|error|event|exit|finally|for|friend|function|get|gettype|gosub|goto|handles|if|implements|imports|in|inherits|integer|interface|is|let|lib|like|long|loop|me|mod|module|mustinherit|mustoverride|mybase|myclass|namespace|new|next|not|notinheritable|notoverridable|object|on|option|optional|or|orelse|overloads|overridable|overrides|paramarray|preserve|private|property|protected|public|raiseevent|readonly|redim|removehandler|resume|return|select|set|shadows|shared|short|single|static|step|stop|string|structure|sub|synclock|then|throw|to|try|typeof|unicode|until|variant|wend|when|while|with|withevents|writeonly|xor|endif|gosub|let|variant|wend)\b/i,
null],["com",/^rem\b.*/i],["lit",/^(?:true\b|false\b|nothing\b|\d+(?:e[+-]?\d+[dfr]?|[dfilrs])?|(?:&h[\da-f]+|&o[0-7]+)[ils]?|\d*\.\d+(?:e[+-]?\d+)?[dfr]?|#\s+(?:\d+[/-]\d+[/-]\d+(?:\s+\d+:\d+(?::\d+)?(\s*(?:am|pm))?)?|\d+:\d+(?::\d+)?(\s*(?:am|pm))?)\s+#)/i],["pln",/^(?:(?:[a-z]|_\w)\w*(?:\[[!#%&@]+])?|\[(?:[a-z]|_\w)\w*])/i],["pun",/^[^\w\t\n\r "'[\]\xa0\u2018\u2019\u201c\u201d\u2028\u2029]+/],["pun",/^(?:\[|])/]]),["vb","vbs"]);

// lang-vhdl.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\t\n\r \xa0]+/,null,"\t\n\r \u00a0"]],[["str",/^(?:[box]?"(?:[^"]|"")*"|'.')/i],["com",/^--[^\n\r]*/],["kwd",/^(?:abs|access|after|alias|all|and|architecture|array|assert|attribute|begin|block|body|buffer|bus|case|component|configuration|constant|disconnect|downto|else|elsif|end|entity|exit|file|for|function|generate|generic|group|guarded|if|impure|in|inertial|inout|is|label|library|linkage|literal|loop|map|mod|nand|new|next|nor|not|null|of|on|open|or|others|out|package|port|postponed|procedure|process|pure|range|record|register|reject|rem|report|return|rol|ror|select|severity|shared|signal|sla|sll|sra|srl|subtype|then|to|transport|type|unaffected|units|until|use|variable|wait|when|while|with|xnor|xor)(?=[^\w-]|$)/i,
null],["typ",/^(?:bit|bit_vector|character|boolean|integer|real|time|string|severity_level|positive|natural|signed|unsigned|line|text|std_u?logic(?:_vector)?)(?=[^\w-]|$)/i,null],["typ",/^'(?:active|ascending|base|delayed|driving|driving_value|event|high|image|instance_name|last_active|last_event|last_value|left|leftof|length|low|path_name|pos|pred|quiet|range|reverse_range|right|rightof|simple_name|stable|succ|transaction|val|value)(?=[^\w-]|$)/i,null],["lit",/^\d+(?:_\d+)*(?:#[\w.\\]+#(?:[+-]?\d+(?:_\d+)*)?|(?:\.\d+(?:_\d+)*)?(?:e[+-]?\d+(?:_\d+)*)?)/i],
["pln",/^(?:[a-z]\w*|\\[^\\]*\\)/i],["pun",/^[^\w\t\n\r "'\xa0][^\w\t\n\r "'\xa0-]*/]]),["vhdl","vhd"]);

// lang-wiki.js
PR.registerLangHandler(PR.createSimpleLexer([["pln",/^[\d\t a-gi-z\xa0]+/,null,"\t \u00a0abcdefgijklmnopqrstuvwxyz0123456789"],["pun",/^[*=[\]^~]+/,null,"=*~^[]"]],[["lang-wiki.meta",/(?:^^|\r\n?|\n)(#[a-z]+)\b/],["lit",/^[A-Z][a-z][\da-z]+[A-Z][a-z][^\W_]+\b/],["lang-",/^{{{([\S\s]+?)}}}/],["lang-",/^`([^\n\r`]+)`/],["str",/^https?:\/\/[^\s#/?]*(?:\/[^\s#?]*)?(?:\?[^\s#]*)?(?:#\S*)?/i],["pln",/^(?:\r\n|[\S\s])[^\n\r#*=A-[^`h{~]*/]]),["wiki"]);
PR.registerLangHandler(PR.createSimpleLexer([["kwd",/^#[a-z]+/i,null,"#"]],[]),["wiki.meta"]);

// lang-yaml.js
var a=null;
var a=null;
PR.registerLangHandler(PR.createSimpleLexer([["pun",/^[:>?|]+/,a,":|>?"],["dec",/^%(?:YAML|TAG)[^\n\r#]+/,a,"%"],["typ",/^&\S+/,a,"&"],["typ",/^!\S*/,a,"!"],["str",/^"(?:[^"\\]|\\.)*(?:"|$)/,a,'"'],["str",/^'(?:[^']|'')*(?:'|$)/,a,"'"],["com",/^#[^\n\r]*/,a,"#"],["pln",/^\s+/,a," \t\r\n"]],[["dec",/^(?:---|\.\.\.)(?:[\n\r]|$)/],["pun",/^-/],["kwd",/^\w+:[\n\r ]/],["pln",/^\w+/]]),["yaml","yml"]);
