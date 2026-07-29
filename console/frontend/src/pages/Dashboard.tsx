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
} from '@patternfly/react-core';
import {
  ChartDonut,
  ChartArea,
  ChartGroup,
  ChartVoronoiContainer,
  ChartThemeColor,
} from '@patternfly/react-charts';
import { safeFetch } from '../utils/api';

interface ClusterInfo {
  name: string;
  region: string;
  status: string;
  agents: number;
  policies: number;
}

interface StatusInfo {
  operatorVersion: string;
  totalAgents: number;
  totalPolicies: number;
  totalEvents: number;
  uptime: string;
}

interface ResourceItem {
  name: string;
  value: number;
}

interface TrendItem {
  date: string;
  count: number;
}

interface ScalingItem {
  date: string;
  applied: number;
  skipped: number;
}

interface StatsInfo {
  resourceDistribution: ResourceItem[];
  eventsTrend: TrendItem[];
  scalingActions: ScalingItem[];
}

export const Dashboard: React.FC = () => {
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [status, setStatus] = useState<StatusInfo | null>(null);
  const [stats, setStats] = useState<StatsInfo | null>(null);

  useEffect(() => {
    safeFetch<ClusterInfo[]>('/api/v1/clusters').then(d => setClusters(d || []));
    safeFetch<StatusInfo>('/api/v1/status').then(d => setStatus(d));
    safeFetch<StatsInfo>('/api/v1/stats').then(d => setStats(d));
  }, []);

  const donutData = stats?.resourceDistribution.map(r => ({ x: r.name, y: r.value })) || [];
  const donutTotal = donutData.reduce((sum, d) => sum + d.y, 0);

  const areaData = stats?.eventsTrend.map(t => ({
    x: t.date.slice(5),
    y: t.count,
  })) || [];

  const appliedData = stats?.scalingActions.map(s => ({
    x: s.date.slice(5),
    y: s.applied,
  })) || [];

  const skippedData = stats?.scalingActions.map(s => ({
    x: s.date.slice(5),
    y: s.skipped,
  })) || [];

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Multi-cluster Governance Dashboard
      </Title>

      {status && (
        <Card style={{ marginBottom: '1rem' }}>
          <CardTitle>Operator Status</CardTitle>
          <CardBody>
            <Flex>
              <FlexItem>
                <DescriptionList isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Version</DescriptionListTerm>
                    <DescriptionListDescription>{status.operatorVersion}</DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Uptime</DescriptionListTerm>
                    <DescriptionListDescription>{status.uptime}</DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </FlexItem>
              <FlexItem>
                <DescriptionList isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Active Agents</DescriptionListTerm>
                    <DescriptionListDescription>{status.totalAgents}</DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Total Policies</DescriptionListTerm>
                    <DescriptionListDescription>{status.totalPolicies}</DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </FlexItem>
              <FlexItem>
                <DescriptionList isHorizontal>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Total Events</DescriptionListTerm>
                    <DescriptionListDescription>{status.totalEvents}</DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </FlexItem>
            </Flex>
          </CardBody>
        </Card>
      )}

      {stats && (
        <Grid hasGutter style={{ marginBottom: '1rem' }}>
          <GridItem span={4}>
            <Card>
              <CardTitle>Resource Distribution</CardTitle>
              <CardBody>
                <div style={{ height: '230px', width: '100%' }}>
                  <ChartDonut
                    data={donutData}
                    title={`${donutTotal}`}
                    subTitle="Total Resources"
                    constrainToVisibleArea
                    labels={({ datum }) => `${datum.x}: ${datum.y}`}
                    legendData={stats.resourceDistribution.map(r => ({ name: `${r.name}: ${r.value}` }))}
                    legendOrientation="vertical"
                    legendPosition="right"
                    padding={{ bottom: 20, left: 20, right: 140, top: 20 }}
                    width={350}
                    height={230}
                    themeColor={ChartThemeColor.multiOrdered}
                  />
                </div>
              </CardBody>
            </Card>
          </GridItem>
          <GridItem span={8}>
            <Card>
              <CardTitle>Scaling Events (Last 7 Days)</CardTitle>
              <CardBody>
                <div style={{ height: '230px', width: '100%' }}>
                  <ChartGroup
                    containerComponent={
                      <ChartVoronoiContainer
                        labels={({ datum }) => `${datum.x}: ${datum.y}`}
                        constrainToVisibleArea
                      />
                    }
                    height={230}
                    padding={{ bottom: 40, left: 50, right: 20, top: 20 }}
                  >
                    <ChartArea
                      data={appliedData}
                      interpolation="monotoneX"
                      name="Applied"
                      style={{ data: { fill: '#06c', fillOpacity: 0.3, stroke: '#06c' } }}
                    />
                    <ChartArea
                      data={skippedData}
                      interpolation="monotoneX"
                      name="Skipped"
                      style={{ data: { fill: '#f0ab00', fillOpacity: 0.3, stroke: '#f0ab00' } }}
                    />
                    <ChartArea
                      data={areaData}
                      interpolation="monotoneX"
                      name="Total Events"
                      style={{ data: { fill: '#009596', fillOpacity: 0.2, stroke: '#009596' } }}
                    />
                  </ChartGroup>
                </div>
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      )}

      <Title headingLevel="h2" size="xl" style={{ marginBottom: '1rem' }}>
        Clusters
      </Title>
      <Grid hasGutter>
        {clusters.map((cluster) => (
          <GridItem key={cluster.name} span={4}>
            <Card>
              <CardTitle>
                <Flex>
                  <FlexItem>{cluster.name}</FlexItem>
                  <FlexItem>
                    <Label color={cluster.status === 'healthy' ? 'green' : 'red'}>
                      {cluster.status}
                    </Label>
                  </FlexItem>
                </Flex>
              </CardTitle>
              <CardBody>
                <DescriptionList>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Region</DescriptionListTerm>
                    <DescriptionListDescription>{cluster.region}</DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Agents</DescriptionListTerm>
                    <DescriptionListDescription>{cluster.agents}</DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Policies</DescriptionListTerm>
                    <DescriptionListDescription>{cluster.policies}</DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
            </Card>
          </GridItem>
        ))}
      </Grid>
    </>
  );
};
