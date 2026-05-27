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

export const Dashboard: React.FC = () => {
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [status, setStatus] = useState<StatusInfo | null>(null);

  useEffect(() => {
    fetch('/api/v1/clusters').then(r => r.json()).then(setClusters);
    fetch('/api/v1/status').then(r => r.json()).then(setStatus);
  }, []);

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
