import React, { useEffect, useState } from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Grid,
  GridItem,
  Title,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Label,
  Flex,
  FlexItem,
  Alert,
  Spinner,
  CodeBlock,
  CodeBlockCode,
} from '@patternfly/react-core';

interface ThanosInfo {
  status: string;
  endpoint: string;
  activeTargets: number;
}

interface OTelInfo {
  status: string;
  endpoint: string;
  protocol: string;
  port: number;
}

interface PipelineInfo {
  name: string;
  receivers: string[];
  exporters: string[];
  status: string;
}

interface ObservabilityData {
  thanos: ThanosInfo;
  opentelemetry: OTelInfo;
  metricsSource: string;
  pipelines: PipelineInfo[];
}

interface MetricResult {
  metric: Record<string, string>;
  value: [number, string];
}

export const Observability: React.FC = () => {
  const [data, setData] = useState<ObservabilityData | null>(null);
  const [metrics, setMetrics] = useState<MetricResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [metricsQuery, setMetricsQuery] = useState('up{job="kubelet"}');

  useEffect(() => {
    fetch('/api/v1/observability')
      .then(r => r.json())
      .then(setData)
      .finally(() => setLoading(false));

    fetch('/api/v1/metrics/query?query=up%7Bjob%3D%22kubelet%22%7D')
      .then(r => r.json())
      .then(d => {
        if (d.data && d.data.result) {
          setMetrics(d.data.result.slice(0, 10));
        }
      });
  }, []);

  if (loading) return <Spinner />;

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Observability & Metrics
      </Title>

      {data && (
        <>
          <Grid hasGutter style={{ marginBottom: '1.5rem' }}>
            <GridItem span={6}>
              <Card>
                <CardTitle>
                  <Flex>
                    <FlexItem>Thanos Querier</FlexItem>
                    <FlexItem>
                      <Label color={data.thanos.status === 'connected' ? 'green' : 'red'}>
                        {data.thanos.status}
                      </Label>
                    </FlexItem>
                  </Flex>
                </CardTitle>
                <CardBody>
                  <DescriptionList>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Endpoint</DescriptionListTerm>
                      <DescriptionListDescription>
                        <code>{data.thanos.endpoint}</code>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Active Targets</DescriptionListTerm>
                      <DescriptionListDescription>{data.thanos.activeTargets}</DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Protocol</DescriptionListTerm>
                      <DescriptionListDescription>PromQL over HTTPS</DescriptionListDescription>
                    </DescriptionListGroup>
                  </DescriptionList>
                </CardBody>
              </Card>
            </GridItem>

            <GridItem span={6}>
              <Card>
                <CardTitle>
                  <Flex>
                    <FlexItem>OpenTelemetry Collector</FlexItem>
                    <FlexItem>
                      <Label color={data.opentelemetry.status === 'connected' ? 'green' : data.opentelemetry.status === 'configured' ? 'blue' : 'grey'}>
                        {data.opentelemetry.status}
                      </Label>
                    </FlexItem>
                  </Flex>
                </CardTitle>
                <CardBody>
                  <DescriptionList>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Endpoint</DescriptionListTerm>
                      <DescriptionListDescription>
                        <code>{data.opentelemetry.endpoint}</code>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Protocol</DescriptionListTerm>
                      <DescriptionListDescription>{data.opentelemetry.protocol}</DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Port</DescriptionListTerm>
                      <DescriptionListDescription>{data.opentelemetry.port}</DescriptionListDescription>
                    </DescriptionListGroup>
                  </DescriptionList>
                </CardBody>
              </Card>
            </GridItem>
          </Grid>

          <Card style={{ marginBottom: '1.5rem' }}>
            <CardTitle>Active Metrics Source</CardTitle>
            <CardBody>
              <Alert
                variant={data.metricsSource !== 'none' ? 'success' : 'warning'}
                isInline
                title={`Kairos is reading metrics from: ${data.metricsSource || 'No source available'}`}
              />
              {data.pipelines && data.pipelines.length > 0 && (
                <div style={{ marginTop: '1rem' }}>
                  <Title headingLevel="h4" size="md">Pipelines</Title>
                  {data.pipelines.map((p, i) => (
                    <DescriptionList key={i} isHorizontal style={{ marginTop: '0.5rem' }}>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Pipeline</DescriptionListTerm>
                        <DescriptionListDescription>
                          <Label color="blue">{p.name}</Label>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Receivers</DescriptionListTerm>
                        <DescriptionListDescription>
                          {p.receivers.map(r => <Label key={r} color="cyan" style={{marginRight: '0.3rem'}}>{r}</Label>)}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Exporters</DescriptionListTerm>
                        <DescriptionListDescription>
                          {p.exporters.map(e => <Label key={e} color="purple" style={{marginRight: '0.3rem'}}>{e}</Label>)}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Status</DescriptionListTerm>
                        <DescriptionListDescription>
                          <Label color="green">{p.status}</Label>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  ))}
                </div>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardTitle>Live Metrics (Thanos Query: <code>up{'{'}job="kubelet"{'}'}</code>)</CardTitle>
            <CardBody>
              {metrics.length > 0 ? (
                <CodeBlock>
                  <CodeBlockCode>
                    {metrics.map((m, i) => {
                      const labels = Object.entries(m.metric)
                        .map(([k, v]) => `${k}="${v}"`)
                        .join(', ');
                      return `${m.metric.__name__ || 'up'}{${labels}} => ${m.value[1]}`;
                    }).join('\n')}
                  </CodeBlockCode>
                </CodeBlock>
              ) : (
                <Alert variant="info" isInline title="Metrics will appear here when Thanos is reachable from the console pod" />
              )}
            </CardBody>
          </Card>
        </>
      )}
    </>
  );
};
