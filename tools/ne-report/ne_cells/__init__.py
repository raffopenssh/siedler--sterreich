"""ne_cells — Nutzungseinheit cells: declared land use + fabric statistics per H3 cell,
computed deterministically from cadastre objects (parcels, footprints, Benützungsart pieces).

Pinned: shapely==2.1.2 (bundled GEOS), h3==4.5.0. Same package + same canonical input
⇒ bit-identical records and digest, whether the input came from the index export or from
a bevdirect tile fetch. See docs/ne-cells.md.
"""
from .algo import ALGO, H3_RES, K_RES, K, build_cells, BuildResult
from .pack import pack_section, unpack_sections, Section, FMT_VER, MAGIC_LU, MAGIC_OBS
from .ns_groups import GROUP_ORDER, NS_TABLE_VERSION, group_of

__version__ = "1.0.0"
__all__ = ["ALGO", "H3_RES", "K_RES", "K", "build_cells", "BuildResult", "pack_section",
           "unpack_sections", "Section", "FMT_VER", "MAGIC_LU", "MAGIC_OBS", "GROUP_ORDER",
           "NS_TABLE_VERSION", "group_of"]
