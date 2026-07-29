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
  ExpandableSection,
  DataList,
  DataListItem,
  DataListItemRow,
  DataListItemCells,
  DataListCell,
} from '@patternfly/react-core';
import { safeFetch } from '../utils/api';

interface RuleDetail {
  name: string;
  type: string;
  actionType: string;
  metric?: string;
  threshold?: string;
  cron?: string;
  cooldown?: string;
}

interface PolicyInfo {
  name: string;
  namespace: string;
  target: string;
  rules: number;
  ruleDetails?: RuleDetail[];
  paused: boolean;
  lastAction: string;
}

export const Policies: React.FC = () => {
  const [policies, setPolicies] = useState<PolicyInfo[]>([]);
  const [expandedPolicies, setExpandedPolicies] = useState<Record<string, boolean>>({});

  useEffect(() => {
    safeFetch<PolicyInfo[]>('/api/v1/policies').then(d => setPolicies(d || []));
  }, []);

  const toggleExpanded = (name: string) => {
    setExpandedPolicies(prev => ({ ...prev, [name]: !prev[name] }));
  };

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
                <DescriptionListDescription>
                  <ExpandableSection
                    toggleText={`${policy.rules} Rule${policy.rules !== 1 ? 's' : ''}`}
                    isExpanded={expandedPolicies[policy.name] || false}
                    onToggle={() => toggleExpanded(policy.name)}
                  >
                    {policy.ruleDetails && policy.ruleDetails.length > 0 ? (
                      <DataList aria-label="Rule details" isCompact>
                        {policy.ruleDetails.map((rule) => (
                          <DataListItem key={rule.name}>
                            <DataListItemRow>
                              <DataListItemCells
                                dataListCells={[
                                  <DataListCell key="name" width={2}>
                                    <strong>{rule.name}</strong>
                                  </DataListCell>,
                                  <DataListCell key="type" width={1}>
                                    <Label color={rule.type === 'metric' ? 'blue' : 'purple'} isCompact>
                                      {rule.type}
                                    </Label>
                                  </DataListCell>,
                                  <DataListCell key="action" width={2}>
                                    {rule.actionType}
                                  </DataListCell>,
                                  <DataListCell key="detail" width={3}>
                                    {rule.type === 'metric'
                                      ? `${rule.metric || ''} ${rule.threshold || ''}`
                                      : rule.cron || ''}
                                    {rule.cooldown ? ` (cooldown: ${rule.cooldown})` : ''}
                                  </DataListCell>,
                                ]}
                              />
                            </DataListItemRow>
                          </DataListItem>
                        ))}
                      </DataList>
                    ) : (
                      <span>No rule details available</span>
                    )}
                  </ExpandableSection>
                </DescriptionListDescription>
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
