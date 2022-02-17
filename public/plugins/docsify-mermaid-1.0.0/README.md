mermaid-docsify is a docsify plugin which allows to render mermaid diagrams in docsify.

## How to use

Add Mermaid and the plugin:

```html
<script src="//unpkg.com/mermaid/dist/mermaid.js"></script>
<script src="mermaid-docsify.js"> <!-- This is not hosted yet>
```

Now you can include mermaid diagrams in your docsify docs:

    ```mermaid
    graph LR
        A --- B
        B-->C[fa:fa-ban forbidden]
        B-->D(fa:fa-spinner);
    ```

![Docsify with mermaid Screenshot](screenshot.png)