// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"io"
	"path"
	"strings"
	"github.com/gogs/git-module"

	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/tool"
)

var mimetypes map[string]string = map[string]string{
	".epub": "application/epub+zip",
	".fif": "application/fractals",
	".spl": "application/futuresplash",
	".hta": "application/hta",
	".hqx": "application/mac-binhex40",
	".vsi": "application/ms-vsi",
	".accdb": "application/msaccess",
	".accda": "application/msaccess.addin",
	".accdc": "application/msaccess.cab",
	".accde": "application/msaccess.exec",
	".accft": "application/msaccess.ftemplate",
	".accdr": "application/msaccess.runtime",
	".accdt": "application/msaccess.template",
	".accdw": "application/msaccess.webapplication",
	".one": "application/msonenote",
	".doc": "application/msword",
	".osdx": "application/opensearchdescription+xml",
	".pdf": "application/pdf",
	".p10": "application/pkcs10",
	".p7c": "application/pkcs7-mime",
	".p7s": "application/pkcs7-signature",
	".cer": "application/pkix-cert",
	".crl": "application/pkix-crl",
	".ps": "application/postscript",
	".xls": "application/vnd.ms-excel",
	".xlsx": "application/vnd.ms-excel.12",
	".xlam": "application/vnd.ms-excel.addin.macroEnabled.12",
	".xlsm": "application/vnd.ms-excel.sheet.macroEnabled.12",
	".xltm": "application/vnd.ms-excel.template.macroEnabled.12",
	".thmx": "application/vnd.ms-officetheme",
	".sst": "application/vnd.ms-pki.certstore",
	".pko": "application/vnd.ms-pki.pko",
	".cat": "application/vnd.ms-pki.seccat",
	".ppt": "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.ms-powerpoint.12",
	".ppam": "application/vnd.ms-powerpoint.addin.macroEnabled.12",
	".pptm": "application/vnd.ms-powerpoint.presentation.macroEnabled.12",
	".sldm": "application/vnd.ms-powerpoint.slide.macroEnabled.12",
	".ppsm": "application/vnd.ms-powerpoint.slideshow.macroEnabled.12",
	".potm": "application/vnd.ms-powerpoint.template.macroEnabled.12",
	".pub": "application/vnd.ms-publisher",
	".vsd": "application/vnd.ms-visio.viewer",
	".docx": "application/vnd.ms-word.document.12",
	".docm": "application/vnd.ms-word.document.macroEnabled.12",
	".dotx": "application/vnd.ms-word.template.12",
	".dotm": "application/vnd.ms-word.template.macroEnabled.12",
	".wpl": "application/vnd.ms-wpl",
	".xps": "application/vnd.ms-xpsdocument",
	".odp": "application/vnd.oasis.opendocument.presentation",
	".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".odt": "application/vnd.oasis.opendocument.text",
	".sldx": "application/vnd.openxmlformats-officedocument.presentationml.slide",
	".ppsx": "application/vnd.openxmlformats-officedocument.presentationml.slideshow",
	".potx": "application/vnd.openxmlformats-officedocument.presentationml.template",
	".xltx": "application/vnd.openxmlformats-officedocument.spreadsheetml.template",
	".appcontent-ms": "application/windows-appcontent+xml",
	".z": "application/x-compress",
	".solitairetheme8": "application/x-compressed",
	".dtcp-ip": "application/x-dtcp1",
	".gz": "application/x-gzip",
	".itls": "application/x-itunes-itls",
	".itms": "application/x-itunes-itms",
	".itpc": "application/x-itunes-itpc",
	".jtx": "application/x-jtx+xps",
	".latex": "application/x-latex",
	".nix": "application/x-mix-transfer",
	".application": "application/x-ms-application",
	".vsto": "application/x-ms-vsto",
	".wmd": "application/x-ms-wmd",
	".wmz": "application/x-ms-wmz",
	".xbap": "application/x-ms-xbap",
	".website": "application/x-mswebsite",
	".p12": "application/x-pkcs12",
	".p7b": "application/x-pkcs7-certificates",
	".p7r": "application/x-pkcs7-certreqresp",
	".pcast": "application/x-podcast",
	".swf": "application/x-shockwave-flash",
	".sit": "application/x-stuffit",
	".tar": "application/x-tar",
	".man": "application/x-troff-man",
	".xaml": "application/xaml+xml",
	".xht": "application/xhtml+xml",
	".xml": "application/xml",
	".zip": "application/zip",
	".3gp": "audio/3gpp",
	".3g2": "audio/3gpp2",
	".aac": "audio/aac",
	".aiff": "audio/aiff",
	".amr": "audio/amr",
	".au": "audio/basic",
	".ec3": "audio/ec3",
	".lpcm": "audio/l16",
	".mid": "audio/mid",
	".mp3": "audio/mp3",
	".m4a": "audio/mp4",
	".m3u": "audio/mpegurl",
	".adts": "audio/vnd.dlna.adts",
	".ac3": "audio/vnd.dolby.dd-raw",
	".wav": "audio/wav",
	".flac": "audio/x-flac",
	".m4r": "audio/x-m4r",
	".mka": "audio/x-matroska",
	".wax": "audio/x-ms-wax",
	".wma": "audio/x-ms-wma",
	".dib": "image/bmp",
	".gif": "image/gif",
	".jpg": "image/jpeg",
	".jps": "image/jps",
	".mpo": "image/mpo",
	".png": "image/png",
	".pns": "image/pns",
	".svg": "image/svg+xml",
	".tif": "image/tiff",
	".dds": "image/vnd.ms-dds",
	".wdp": "image/vnd.ms-photo",
	".emf": "image/x-emf",
	".ico": "image/x-icon",
	".wmf": "image/x-wmf",
	".dwfx": "model/vnd.dwfx+xps",
	".easmx": "model/vnd.easmx+xps",
	".edrwx": "model/vnd.edrwx+xps",
	".eprtx": "model/vnd.eprtx+xps",
	".ics": "text/calendar",
	".css": "text/css",
	".vcf": "text/directory",
	".htm": "text/html",
	".html": "text/html",
	".txt": "text/plain",
	".wsc": "text/scriptlet",
	".htc": "text/x-component",
	".contact": "text/x-ms-contact",
	".iqy": "text/x-ms-iqy",
	".odc": "text/x-ms-odc",
	".rqy": "text/x-ms-rqy",
	".3gpp": "video/3gpp",
	".3gp2": "video/3gpp2",
	".avi": "video/avi",
	".mp4": "video/mp4",
	".mpeg": "video/mpeg",
	".mov": "video/quicktime",
	".uvu": "video/vnd.dece.mp4",
	".tts": "video/vnd.dlna.mpeg-tts",
	".wtv": "video/wtv",
	".m4v": "video/x-m4v",
	".mkv": "video/x-matroska",
	".asx": "video/x-ms-asf",
	".dvr-ms": "video/x-ms-dvr",
	".wm": "video/x-ms-wm",
	".wmv": "video/x-ms-wmv",
	".wmx": "video/x-ms-wmx",
	".wvx": "video/x-ms-wvx",
	".apk": "application/vnd.android.package-archive",
	".obb": "application/vnd.android.obb",
	".json": "application/json"}

func ServeData(c *context.Context, name string, reader io.Reader) error {
	buf := make([]byte, 1024)
	n, _ := reader.Read(buf)
	if n >= 0 {
		buf = buf[:n]
	}


	if !tool.IsTextFile(buf) {
		if !tool.IsImageFile(buf) {
			c.Resp.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
			c.Resp.Header().Set("Content-Transfer-Encoding", "binary")
		}
	} else if !setting.Repository.EnableRawFileRenderMode {
		c.Resp.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		ext := path.Ext(name)
		ext = strings.ToLower(ext)
		mimetype, ok := mimetypes[ext]
		if ok {
			c.Resp.Header().Set("Content-Type", mimetype)
		}
	}
	c.Resp.Write(buf)
	_, err := io.Copy(c.Resp, reader)
	return err
}

func ServeBlob(c *context.Context, blob *git.Blob) error {
	dataRc, err := blob.Data()
	if err != nil {
		return err
	}

	return ServeData(c, path.Base(c.Repo.TreePath), dataRc)
}

func SingleDownload(c *context.Context) {
	blob, err := c.Repo.Commit.GetBlobByPath(c.Repo.TreePath)
	if err != nil {
		if git.IsErrNotExist(err) {
			c.Handle(404, "GetBlobByPath", nil)
		} else {
			c.Handle(500, "GetBlobByPath", err)
		}
		return
	}
	if err = ServeBlob(c, blob); err != nil {
		c.Handle(500, "ServeBlob", err)
	}
}
