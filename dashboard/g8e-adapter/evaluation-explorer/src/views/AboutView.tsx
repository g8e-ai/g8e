// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import {
  ABOUT_AVAILABILITY_DETAIL,
  ABOUT_AVAILABILITY_HEADING,
  ABOUT_AVAILABILITY_LEDE_AFTER,
  ABOUT_AVAILABILITY_LEDE_BEFORE,
  ABOUT_AVAILABILITY_LEDE_MIDDLE,
  ABOUT_BACKGROUND,
  ABOUT_DELIVER_HEADING,
  ABOUT_DELIVER_INTRO,
  ABOUT_DELIVERABLES,
  ABOUT_EXPERIENCE,
  ABOUT_HEADING,
  ABOUT_LEADERSHIP,
  ABOUT_OPENING_AFTER,
  ABOUT_OPENING_BEFORE,
  ABOUT_SITE_INTRO_AFTER,
  ABOUT_SITE_INTRO_BEFORE,
  G8E_REPO_URL,
  PLATFORM_CONTACT_CALENDLY,
  PLATFORM_CONTACT_EMAIL,
} from '../content/platform';
import { StreamStatusIndicator } from '../components/shared';
import { useConnection, useStoreState } from '../state/store';

export function AboutView() {
  const connection = useConnection();
  const streamConnection = useStoreState((state) => state.streamConnection);

  return (
    <div className="about">
      <section className="panel about-panel" aria-label="About this deployment">
        <div className="panel-head">
          <h2>About</h2>
          <StreamStatusIndicator
            streamConnection={streamConnection}
            feedConnection={connection}
          />
        </div>

        <div className="sys-platform">
          <p className="sys-platform-lede">
            {ABOUT_SITE_INTRO_BEFORE}
            <strong>g8e</strong>
            {ABOUT_SITE_INTRO_AFTER}
          </p>

          <hr className="about-divider" />

          <h3 className="sys-platform-heading">{ABOUT_HEADING}</h3>
          <p className="sys-platform-lede">
            {ABOUT_OPENING_BEFORE}
            <strong>Danny Barbour</strong>
            {ABOUT_OPENING_AFTER}
          </p>
          <p className="sys-platform-lede">{ABOUT_BACKGROUND}</p>
          <p className="sys-platform-lede">{ABOUT_LEADERSHIP}</p>
          <ul className="about-highlights" aria-label="Career highlights">
            {ABOUT_EXPERIENCE.map((item) => (
              <li key={item.company}>
                <strong>{item.company}</strong>, {item.text}
              </li>
            ))}
          </ul>

          <hr className="about-divider" />

          <h3 className="sys-platform-heading">{ABOUT_DELIVER_HEADING}</h3>
          <p className="sys-platform-lede">{ABOUT_DELIVER_INTRO}</p>
          <ul className="about-highlights" aria-label="Areas of practice">
            {ABOUT_DELIVERABLES.map((item) => (
              <li key={item.label}>
                <strong>{item.label}:</strong> {item.detail}
              </li>
            ))}
          </ul>

          <hr className="about-divider" />

          <h3 className="sys-platform-heading">{ABOUT_AVAILABILITY_HEADING}</h3>
          <p className="sys-platform-lede about-availability">
            {ABOUT_AVAILABILITY_LEDE_BEFORE}
            <strong>contract engagements</strong>
            {ABOUT_AVAILABILITY_LEDE_MIDDLE}
            <strong>full-time</strong>
            {ABOUT_AVAILABILITY_LEDE_AFTER}
          </p>
          <p className="sys-platform-lede about-availability">{ABOUT_AVAILABILITY_DETAIL}</p>
          <p className="sys-platform-lede about-availability">
            Reach me at{' '}
            <a href={`mailto:${PLATFORM_CONTACT_EMAIL}`}>{PLATFORM_CONTACT_EMAIL}</a>.
          </p>

          <div className="sys-platform-cta">
            <div className="sys-platform-links">
              <a
                className="sys-platform-cta-architecture"
                href={PLATFORM_CONTACT_CALENDLY}
                target="_blank"
                rel="noopener noreferrer"
              >
                Book a call with Calendly
              </a>
              <a
                className="sys-platform-cta-architecture"
                href={G8E_REPO_URL}
                target="_blank"
                rel="noopener noreferrer"
              >
                Explore the g8e Codebase on GitHub
              </a>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
