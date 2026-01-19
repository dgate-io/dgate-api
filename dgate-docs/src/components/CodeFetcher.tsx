// create a component that fetches the latest release from a Github repository
// using this url: https://api.github.com/repos/dgate-io/dgate/releases/latest

import React, { useEffect } from "react";
import { useState } from "react";
import CodeBlock from '@theme/CodeBlock';

type Props = {
    url: string;
    title: string;
};

function CodeFetcher({ url, title }: Props) {
    const [data, setData] = useState<string | null>(null);
    const [retry, setRetry] = useState(0);

    useEffect(() => {
        fetch(url)
            .then((response) => response.text())
            .then((data) => setData(data))
            .catch((error) => {
                console.error(error);
                setTimeout(() => setRetry(retry + 1), 3000)
            });
    }, [url]);

    if (retry > 10) {
        return null;
    }

    return (data) ? <CodeBlock title={title} language="typescript">{data}</CodeBlock> :
    <h1 className="box-progress" data-text="Loading">
        Loading...
    </h1>;
}

export default CodeFetcher