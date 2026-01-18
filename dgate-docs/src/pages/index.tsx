import React, { useEffect } from 'react';
import clsx from 'clsx';
import mermaid from 'mermaid';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import { Analytics } from '@vercel/analytics/react';
import Link from '@docusaurus/Link';

import styles from './index.module.css';
import { Button,
  Container, extendTheme,
  HStack,
  useColorMode as useChakraColor,
  withDefaultColorScheme,
  ChakraBaseProvider,
  Spacer,
  Flex,
  Center,
  Text,
  Divider,
  Wrap,
  ColorModeProvider,
} from '@chakra-ui/react';
import { useColorMode as useDocColor } from '@docusaurus/theme-common';

mermaid.initialize({
  startOnLoad: true,
  theme: 'dark',
  securityLevel: 'loose',
  fontFamily: 'Nerd Font, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Oxygen, Ubuntu, Cantarell, Fira Sans, Droid Sans, Helvetica Neue, sans-serif',
  fontSize: 14,
});

if (typeof window !== 'undefined') {
  const uid = localStorage.getItem('uid') || crypto.randomUUID();
  const sid = sessionStorage.getItem('sid') || crypto.randomUUID();
  localStorage.setItem('uid', uid);
  localStorage.setItem('sid', sid);
  const logger = () => {
    const eid = crypto.randomUUID();
    window.navigator.sendBeacon(`https://events.dgate.cloud/log?etype=webeventv1&eid=${eid}`, JSON.stringify({
      event: 'pageview',
      event_id: eid,
      user_id: uid,
      session_id: sid,
      url: window.location.href,
      path: window.location.pathname,
      language: navigator.language,
      referrer: document.referrer,
      user_agent: navigator.userAgent,
      screen_width: window.screen.width,
      screen_height: window.screen.height,
      viewport_width: window.innerWidth,
      viewport_height: window.innerHeight,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      timestamp: new Date().toISOString(),
    }));
  };

  if (window['navigation']) {
    window['navigation']?.addEventListener('navigate', logger)
  } else {
    window.addEventListener('load', logger);
  }
}

const customTheme = extendTheme({
  ...withDefaultColorScheme({
    colorScheme: 'red',
  }),
  colors: {
    primaryFontColor: {
      lightMode: 'black',
      darkMode: 'white',
    }, 
    secondaryFontColor: {
      lightMode: 'gray.600',
      darkMode: 'gray.300',
    },
  },
});

const features = [
  {
    title: 'Manage using CLI',
    description: 'DGate offers a CLI tool to allow users to manage resources using the Admin API. Monitoring, debugging, verifying changes, and deploying resources can be done using the CLI tool.',
  },
  {
    title: 'Distributed Replication',
    description: 'DGate supports replication of resources across multiple instances using the Raft Consensus Algorithm. This allows the user to scale the gateway horizontally and ensure high availability.',
  },
  {
    title: 'Typescript Functions',
    description: 'Modules support JavaScript and TypeScript functions. This allows the user to write custom functions to manipulate the request and response objects.',
  },
];

const roadmap = [
  {
    title: 'WAF',
    description: 'DGate plans to offer support for Web Application Firewall (WAF) to protect resources from common web attacks Users will be able to update/change these in real-time with no updates.',
  },
  {
    title: 'Resource Canary',
    description: 'DGate plans to offer support for canary deployments to allow users to test new features in production with a subset of users.',
  },
  {
    title: 'Admin Dashboard',
    description: 'DGate plans to offer an admin dashboard to allow users to manage resources from a web interface. The web interface will also offer monitoring and module debugging tools.',
  },
];

export default function HomePage(): JSX.Element {
  const {siteConfig} = useDocusaurusContext();
  return (
    <ChakraBaseProvider theme={customTheme}>
      <ColorModeProvider>
        <Layout title={siteConfig.title}>
          <Analytics />
          <Hero siteConfig={siteConfig} />
        </Layout>
      </ColorModeProvider>
    </ChakraBaseProvider>
  );
}

function Hero({ siteConfig }) {
  const { setColorMode } = useChakraColor();
  const { colorMode } = useDocColor();

  useEffect(() => {
    setColorMode(colorMode);
  }, [colorMode])

  return (
    <header>
    <div className={clsx(styles.jumbo, styles.heroBanner)}>
      <img className={styles.jumboImg} src="/img/dgate.svg" alt="logo" />
    </div>
    <div className={clsx(styles.heroBanner)}>
      <Container>
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <Center>
        <Flex minW="360px" justify="space-evenly">
          <Button as={Link} variant='ghost' href='/docs/getting-started/dgate-server' size='lg'>
            Get Started
          </Button>
          <Spacer />
          <Button as={Link} href='https://github.com/dgate-io/dgate' variant='outline' left={"auto"} size='lg'>
            ⭐&nbsp; on Github
          </Button>
        </Flex>
        </Center>
      </Container>
    </div>
    <Container maxW="5xl" centerContent>
      <Wrap pt='4em' pb='1em'>
        <Heading as='h1'>Features</Heading>
      </Wrap>
      <HStack flexWrap={"wrap"} align={"start"} justify={"space-evenly"} mb="1em">
        {features && features.map((feature, idx) => (
          <Flex key={idx} maxW={"300px"} direction={"column"}>
            <Text fontWeight={"bold"} textAlign={"center"} fontSize={"24px"}>{feature.title}</Text>
            <Text textAlign={"center"}>{feature.description}</Text>
          </Flex>
        ))}
      </HStack>
      <Divider />

      <Wrap pt='4em' pb='1em'>
        <Heading as='h1'>Roadmap Features</Heading>
      </Wrap>
      <HStack flexWrap={"wrap"} align={"start"} justify={"space-evenly"} mb="1em">
        {roadmap && roadmap.map((feature, idx) => (
          <Flex key={idx} maxW={"300px"} direction={"column"}>
            <Text fontWeight={"bold"} textAlign={"center"} fontSize={"24px"}>{feature.title}</Text>
            <Text textAlign={"center"}>{feature.description}</Text>
          </Flex>
        ))}
      </HStack>
    </Container>
  </header>
  );
}
