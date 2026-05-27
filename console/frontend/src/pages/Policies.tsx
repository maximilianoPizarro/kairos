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

interface PolicyInfo {
  name: string;
  namespace: string;
  target: string;
  rules: number;
  paused: boolean;
  lastAction: string;
}

export const Policies: React.FC = () => {
  const [policies, setPolicies] = useState<PolicyInfo[]>([]);

  useEffect(() => {
    fetch('/api/v1/policies').then(r => r.json()).then(setPolicies);
  }, []);

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Scaling Policies
      </Title>

      {policies.map((policy) => (
        <Card key={policy.name} style={{ marginBottom: '1rem' }}>
          <CardTitle>
            {policy.name}
            <Label color={policy.paused ? 'grey' : 'green'} style={{ marginLeft: '0.5rem' }}>
              {policy.paused ? 'Paused' : 'Active'}
            </Label>
          </CardTitle>
          <CardBody>
            <DescriptionList isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Namespace</DescriptionListTerm>
                <DescriptionListDescription>{policy.namespace}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Target</DescriptionListTerm>
                <DescriptionListDescription>{policy.target}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Rules</DescriptionListTerm>
                <DescriptionListDescription>{policy.rules}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Last Action</DescriptionListTerm>
                <DescriptionListDescription>{new Date(policy.lastAction).toLocaleString()}</DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </CardBody>
        </Card>
      ))}
    </>
  );
};
