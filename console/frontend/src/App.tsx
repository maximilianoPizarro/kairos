import React, { useState } from 'react';
import {
  Page,
  Masthead,
  MastheadMain,
  MastheadBrand,
  MastheadContent,
  PageSidebar,
  PageSidebarBody,
  PageSection,
  Nav,
  NavList,
  NavItem,
  Brand,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Label,
} from '@patternfly/react-core';
import { Dashboard } from './pages/Dashboard';
import { Agents } from './pages/Agents';
import { Policies } from './pages/Policies';
import { Events } from './pages/Events';
import { Observability } from './pages/Observability';
import { ManagedResources } from './pages/ManagedResources';

type PageKey = 'dashboard' | 'agents' | 'policies' | 'events' | 'observability' | 'resources';

export const App: React.FC = () => {
  const [activePage, setActivePage] = useState<PageKey>('dashboard');

  const renderPage = () => {
    switch (activePage) {
      case 'dashboard': return <Dashboard />;
      case 'agents': return <Agents />;
      case 'policies': return <Policies />;
      case 'events': return <Events />;
      case 'observability': return <Observability />;
      case 'resources': return <ManagedResources />;
      default: return <Dashboard />;
    }
  };

  const header = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <Title headingLevel="h1" size="xl">Kairos</Title>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Label color="green">Operator v0.1.0</Label>
            </ToolbarItem>
            <ToolbarItem>
              <Label color="blue">3 Clusters</Label>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>
        <Nav>
          <NavList>
            <NavItem isActive={activePage === 'dashboard'} onClick={() => setActivePage('dashboard')}>
              Dashboard
            </NavItem>
            <NavItem isActive={activePage === 'agents'} onClick={() => setActivePage('agents')}>
              AI Agents
            </NavItem>
            <NavItem isActive={activePage === 'policies'} onClick={() => setActivePage('policies')}>
              Scaling Policies
            </NavItem>
            <NavItem isActive={activePage === 'events'} onClick={() => setActivePage('events')}>
              Events
            </NavItem>
            <NavItem isActive={activePage === 'observability'} onClick={() => setActivePage('observability')}>
              Observability
            </NavItem>
            <NavItem isActive={activePage === 'resources'} onClick={() => setActivePage('resources')}>
              Managed Resources
            </NavItem>
          </NavList>
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  return (
    <Page header={header} sidebar={sidebar}>
      <PageSection>
        {renderPage()}
      </PageSection>
    </Page>
  );
};
