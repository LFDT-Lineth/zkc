;; As lookup_15, but without selectors.
(defcolumns (X :i16) (Y :i16) (PX :i16))
;; PX holds the value of the expression (shift X -1)
(defconstraint px_def () (== PX (shift X -1)))
(deflookup test ((_ PX)) ((_ Y)))
