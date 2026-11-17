// https://originui.com/r/comp-23.json

'use client';

import React, { useId } from 'react';
import { Input, InputProps } from '../ui/input';

export default React.forwardRef<HTMLInputElement, InputProps>(
  function PasswordInput({ ...props }, ref) {
    const id = useId();

    return (
      <div className="*:not-first:mt-2">
        <div className="relative">
          <Input
            id={id}
            className="pe-9"
            placeholder="Password"
            type="password"
            ref={ref}
            {...props}
          />
        </div>
      </div>
    );
  },
);
