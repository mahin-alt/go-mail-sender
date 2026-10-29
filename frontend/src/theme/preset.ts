import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

/**
 * PrimeVue theme for MailFlow (decision 0007): Aura with a black/zinc primary so buttons, focus
 * rings and selected states match the ink colour. Component-level tweaks are added here, per
 * component, when a phase first uses that component, never with Tailwind colour classes.
 */
export const MailFlowPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{zinc.50}',
      100: '{zinc.100}',
      200: '{zinc.200}',
      300: '{zinc.300}',
      400: '{zinc.400}',
      500: '{zinc.500}',
      600: '{zinc.600}',
      700: '{zinc.700}',
      800: '{zinc.800}',
      900: '{zinc.900}',
      950: '{zinc.950}',
    },
    colorScheme: {
      light: {
        primary: {
          color: '#161616',
          contrastColor: '#ffffff',
          hoverColor: '{zinc.800}',
          activeColor: '{zinc.700}',
        },
      },
    },
  },
})
