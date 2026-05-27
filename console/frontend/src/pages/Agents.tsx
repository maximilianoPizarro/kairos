import React, { useEffect, useState } from 'react';
import {
  Card,
  CardTitle,
  CardBody,
  Title,
  Label,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
} from '@patternfly/react-core';

interface AgentInfo {
  name: string;
  namespace: string;
  mode: string;
  phase: string;
  watchedResources: number;
  totalCorrections: number;
  lastCheck: string;
}

export const Agents: React.FC = () => {
  const [agents, setAgents] = useState<AgentInfo[]>([]);

  useEffect(() => {
    fetch('/api/v1/agents').then(r => r.json()).then(setAgents);
  }, []);

  const phaseColor = (phase: string) => {
    switch (phase) {
      case 'Active': return 'green';
      case 'Correcting': return 'orange';
      case 'WaitingApproval': return 'blue';
      case 'Error': return 'red';
      case 'Paused': return 'grey';
      default: return 'grey';
    }
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        AI Agents
      </Title>

      {agents.map((agent) => (
        <Card key={agent.name} style={{ marginBottom: '1rem' }}>
          <CardTitle>
            {agent.name}
            <Label color={phaseColor(agent.phase)} style={{ marginLeft: '0.5rem' }}>
              {agent.phase}
            </Label>
            <Label color="blue" style={{ marginLeft: '0.5rem' }}>
              {agent.mode}
            </Label>
          </CardTitle>
          <CardBody>
            <DescriptionList isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Namespace</DescriptionListTerm>
                <DescriptionListDescription>{agent.namespace}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Watched Resources</DescriptionListTerm>
                <DescriptionListDescription>{agent.watchedResources}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Total Corrections</DescriptionListTerm>
                <DescriptionListDescription>{agent.totalCorrections}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Last Check</DescriptionListTerm>
                <DescriptionListDescription>{new Date(agent.lastCheck).toLocaleString()}</DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </CardBody>
        </Card>
      ))}
    </>
  );
};
