# Import locales from Crowdin

1. Upload the latest version of [`locale_en-US.ini`](https://github.com/gogs/gogs/blob/main/conf/locale/locale_en-US.ini) to the [Crowdin](https://crowdin.gogs.io/project/gogs/sources/files).
1. [Build and download](https://crowdin.gogs.io/project/gogs/translations) the ZIP archive and unzip it.
1. Go to root directory of the repository.
1. Run the `import` subcommand:

    ```
    $ ./gogs import locale --source <path to the unzipped directory> --target ./conf/locale
    Locale files has been successfully imported!
    ```

1. Run `task web` to start the web server, then visit the site in the browser to make sure nothing blows up.
1. Check out a new branch using `git checkout -b update-locales`.
1. Stage changes
1. Run `git commit -m "locale: sync from Crowdin"`.
1. Push the commit then open up a pull request on GitHub.
