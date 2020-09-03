# Import locales from Crowdin

1. Download the ZIP archive from [Crowdin](https://crowdin.gogs.io/) and unzip it.
1. Go to root directory of the repository.
1. Run the `import` subcommand:

    ```
    $ ./gogs import locale --source <path to the unzipped directory> --target ./conf/locale
    Locale files has been successfully imported!
    ```

1. Run `task generate` to generate corresponding bindata.
1. Run `task web` to start the web server, then visit the site in the browser to make sure nothing blows up.
1. Check out a new branch using `git checkout -b update-locales`.
1. Stash changes then run `git commit -m "locale: sync from Crowdin"`.
1. Push the commit then open up a pull request on GitHub.
