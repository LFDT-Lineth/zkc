(defcolumns (X :i16) (P :binary) (Y :i16) (one :i1 :constant 1))
;; use of selector
(deflookup test ((P X)) ((one Y)))
