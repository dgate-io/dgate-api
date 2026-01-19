// create a component that fetches the latest release from a Github repository
// using this url: https://api.github.com/repos/dgate-io/dgate/releases/latest

import { Text } from "@chakra-ui/react";
import { Spinner } from "@chakra-ui/spinner";
import React, { PropsWithChildren, useEffect } from "react";
import { useState } from "react";

interface Release {
    url:              string;
    assets_url:       string;
    upload_url:       string;
    html_url:         string;
    id:               number;
    tag_name:         string;
    name:             string;
    draft:            boolean;
    prerelease:       boolean;
    created_at:       string;
    published_at:     string;
    assets?:           ReleaseAsset[];
    tarball_url:      string;
    zipball_url:      string;
}

interface ReleaseAsset {
    url:                  string;
    id:                   number;
    name:                 string;
    label:                string;
    content_type:         string;
    state:                string;
    size:                 number;
    download_count:       number;
    created_at:           string;
    updated_at:           string;
    browser_download_url: string;
}

type Props = { };

function GithubReleaseFetcher({ children }: PropsWithChildren<Props>) {
    const [release, setRelease] = useState<Release | null>(null);
    const [retry, setRetry] = useState(0)

    useEffect(() => {
        if (release || retry > 10) return;
        fetch("https://api.github.com/repos/dgate-io/dgate/releases/latest", {
            headers: { "X-GitHub-Api-Version":"2022-11-28" },
        })
            .then((response) => response.json())
            .then((data) => setRelease(data as Release))
            .catch((error) => {
                console.error(error);
                setTimeout(() => setRetry(retry + 1), 3000)
            });
    }, [retry]);

    if (retry > 10) {
        return null;
    }

    return (release) ? (
        <div>
            {children}
            <h3 title={release.name}>Latest release: {release.tag_name} (<span title={release.published_at}>{prettyTime(release.published_at)}</span>)</h3>
            <ul>
                {release?.assets?.filter(asset => asset.name.startsWith("dgate_")).map((asset) => (
                    <li key={asset.id}>
                        <a href={asset.browser_download_url}>{asset.name}</a>
                    </li>
                ))}
            </ul>
        </div>
    ) : <h1 className="box-progress" data-text="Loading">Loading...</h1>;
}

function prettyTime(time: string) {
    var date = new Date((time || "").replace(/-/g,"/").replace(/[TZ]/g," ")),
      diff = (((new Date()).getTime() - date.getTime()) / 1000),
      day_diff = Math.floor(diff / 86400);
    
    // return date for anything greater than a day
    if ( isNaN(day_diff) || day_diff < 0 || day_diff > 0 )
      return date.getDate() + " " + date.toDateString().split(" ")[1];

    return day_diff == 0 && (
        diff < 60 && "just now" ||
        diff < 120 && "1 minute ago" ||
        diff < 3600 && Math.floor( diff / 60 ) + " minutes ago" ||
        diff < 7200 && "1 hour ago" ||
        diff < 86400 && Math.floor( diff / 3600 ) + " hours ago") ||
      day_diff == 1 && "Yesterday" ||
      day_diff < 7 && day_diff + " days ago" ||
      day_diff < 31 && Math.ceil( day_diff / 7 ) + " weeks ago";
}

export default GithubReleaseFetcher