;; As lookup_22, but without a target selector.
(defcolumns (X :i16) (P :binary) (Y :i16))
;; use of selector
(deflookup test ((P X)) ((_ Y)))
