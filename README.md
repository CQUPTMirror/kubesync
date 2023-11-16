# kubesync

## Getting Started

### 1. Deploy controller

```shell
kubectl apply -k config/default
```

### 2. Deploy test manager and first mirror job

```shell
kubectl apply -k config/samples
```

### 3. Deploy more mirror jobs

**For detailed configuration and examples, see the `config` directory**

**You can find most of the CQUPT OpenSource Mirror's configuration files in the `docs/examples` directory**

## Design

```
      +-------------------------------------------------------------------------+
      |                                 API Server                              |
      +---+--^------------------------------------------------+-^---------------+
          |  |                                 Resources Info | | Update Status
          |  |                     +--------------------------+-+---------------------------------------------------------------+
          |  |                     | Namespace #1             | |                                                               |
          |  |                     |                          | | Change Resources                                              |
          |  |                     |                     +----v-+----+                                                          |
          |  |                     |          +--------->|           +------------------------------------------------+         |
          |  |                     |   Update |          |  Manager  |                                                |         |
          |  |                     |   Status | +--------+           +--------------------------------+               |         |
          |  | Manage              |          | |Control +-----------+                                |               |         |
   Handle |  | Resources           |          | |Command                                              | Status        | REST    |
   CRDs   |  | (Deploy/CM/PVC/...) | +--------+-+---------------------------------------------+       | API           | API     |
(Manager/ |  |                     | | Job #1 | |                                             |       |               |         |
 Worker/  |  | Update              | | +------+-v-+ +----------------+ +--------------------+ | +-----v-------+ +-----v-------+ |
Announce/ |  | Deploy              | | |  Worker  | |  Rsync Server  | |  HTTP File Server  | | |  Home Page  | |  Dashboard  | |
 File)    |  | Status              | | +----------+ +--+-------------+ +-+-------------+----+ | +-----+-------+ +-----+-------+ |
          |  |                     | |                 |                 |             |      |       |               |         |
      +---v--+-------+             | +-----------------+-----------------+-------------+------+       |               |         |
      |  Controller  |             |                   |                 |             |              |               |         |
      +--------------+             +-------------------+-----------------+-------------+--------------+---------------+---------+
                                                       |                 | metrics     |              |               |
                                          +------------v--+    +---------v----+    +---v--------------v---------------v---------+
                                          |  Rsync Proxy  |    |  Prometheus  |<---+                   Ingress                  |
                                          +---------------+    +--------------+    +--------------------------------------------+
```

For Chinese design documents, see [docs/design.md](https://github.com/CQUPTMirror/kubesync/blob/master/docs/design.md)

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
