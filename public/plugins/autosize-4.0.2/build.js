var pkg = require('./package.json');
var fs = require('fs');
var ugly = require('uglify-js');
var jshint = require('jshint').JSHINT;
var babel = require('babel-core');
var gaze = require('gaze');

function lint(full) {
	jshint(full.toString(), {
		browser: true,
		undef: true,
		unused: true,
		immed: true,
		eqeqeq: true,
		eqnull: true,
		noarg: true,
		immed: false,
		predef: ['define', 'module', 'exports', 'Map']
	});

	if (jshint.errors.length) {
		jshint.errors.forEach(function (err) {
			console.log(err.line+':'+err.character+' '+err.reason);
		});
	} else {
		console.log('linted')
	}

	return true;
}

function build(code) {
	var minified = ugly.minify(code).code;
	var header = [
		`/*!`,
		`	${pkg.name} ${pkg.version}`,
		`	license: ${pkg.license}`,
		`	${pkg.homepage}`,
		`*/`,
		``
	].join('\n');

	fs.writeFile('dist/'+pkg.name+'.js', header+code);
	fs.writeFile('dist/'+pkg.name+'.min.js', header+minified);
	console.log('dist built');
}

function transform(filepath) {
	babel.transformFile(filepath, {
		plugins: [
			"add-module-exports", ["transform-es2015-modules-umd", {"strict": true, "noInterop": true}]
		],
		presets: ["env"],
	}, function (err,res) {
		if (err) {
			return console.log(err);
		} else {
			lint(res.code);
			build(res.code);
		}
	});
}

gaze('src/'+pkg.name+'.js', function(err, watcher){
	// On file changed
	this.on('changed', function(filepath) {
		transform(filepath);
	});

	console.log('watching');
});

transform('src/'+pkg.name+'.js');