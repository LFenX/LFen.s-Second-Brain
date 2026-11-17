import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { FormFieldType, generateSchema, RenderField } from '../dynamic-form';

jest.mock('lucide-react', () => ({}));
jest.mock('../edit-tag', () => () => null);
jest.mock('../originui/select-with-search', () => ({
  SelectWithSearch: () => null,
}));
jest.mock('../ragflow-form', () => ({ RAGFlowFormItem: () => null }));
jest.mock('../ui/form', () => ({
  Form: jest.requireActual('react-hook-form').FormProvider,
}));
jest.mock('../ui/checkbox', () => ({ Checkbox: () => null }));
jest.mock('../ui/input', () => ({ Input: () => null }));
jest.mock('../ui/textarea', () => ({ Textarea: () => null }));
jest.mock('../ui/multi-select', () => ({ MultiSelect: () => null }));
jest.mock('../ui/segmented', () => ({ Segmented: () => null }));
jest.mock('../ui/switch', () => ({ Switch: () => null }));

describe('dynamic form field schema', () => {
  it('validates deeply nested fields as nested objects', () => {
    const schema = generateSchema([
      {
        name: 'profile.contact.email',
        label: 'Email',
        type: FormFieldType.Email,
        required: true,
      },
    ]);
    expect(
      schema.safeParse({
        profile: { contact: { email: 'a@fixtures.invalid' } },
      }).success,
    ).toBe(true);
    expect(
      schema.safeParse({ profile: { contact: { email: 'invalid' } } }).success,
    ).toBe(false);
  });

  it('rejects a field that conflicts with an existing nested parent', () => {
    expect(() =>
      generateSchema([
        { name: 'profile.contact', label: 'Contact', type: FormFieldType.Text },
        {
          name: 'profile.contact.email',
          label: 'Email',
          type: FormFieldType.Email,
        },
      ]),
    ).toThrow('Conflicting nested field path');
  });

  it('provides real form value and change handlers to custom hidden-label fields', () => {
    function CustomForm() {
      const form = useForm({ defaultValues: { title: 'initial' } });
      return (
        <FormProvider {...form}>
          <RenderField
            field={{
              name: 'title',
              label: 'Title',
              type: FormFieldType.Custom,
              hideLabel: true,
              render: (field) => <input aria-label="Custom title" {...field} />,
            }}
          />
        </FormProvider>
      );
    }
    render(<CustomForm />);
    expect(screen.getByLabelText('Custom title')).toHaveValue('initial');
    fireEvent.change(screen.getByLabelText('Custom title'), {
      target: { value: 'updated' },
    });
    expect(screen.getByLabelText('Custom title')).toHaveValue('updated');
  });
});
