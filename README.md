# kubesync

## Getting Started

1. Deploy controller

```shell
kubectl apply -k config/default
```

2. Deploy test manager and first mirror job

```shell
kubectl apply -k config/samples
```

3. Deploy more mirror jobs

**For detailed configuration and examples, see the `config` directory**

## Credits

- [tunasync](https://github.com/tuna/tunasync): The foundation and inspiration of this project, most of the code of managers and workers is copied or modified from the project.
- [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder): The controller framework used in this project

## License

Copyright (C) 2023  CQUPTMirror

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
