import Details from '@theme/MDXComponents/Details';
import { PropsWithChildren } from 'react';

type Props = {
    title?: string;
};

export default function Collapse (props: PropsWithChildren<Props>) {
    const { children, title } = props;

    return (
        <Details>
            {title ? <summary>{title}</summary> : null}
            {children}
        </Details>
    );
}