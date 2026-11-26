(defcolumns (P :binary) (X :i16) (Q :binary) (Y :i16))
;; use of selector
(deflookup test ((Q X)) ((P Y)))
